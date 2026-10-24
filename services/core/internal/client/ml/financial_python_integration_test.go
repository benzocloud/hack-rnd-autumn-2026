//go:build mlintegration

package ml

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/service/forecasting"
)

func TestPythonFinancialPredictMTLS(t *testing.T) {
	python, source := os.Getenv("CORE_ML_TEST_PYTHON"), os.Getenv("CORE_ML_TEST_SOURCE")
	if python == "" || source == "" {
		t.Skip("set CORE_ML_TEST_PYTHON and CORE_ML_TEST_SOURCE")
	}
	tlsConfig, certs := testTLS(t)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	script := `import pathlib, sys
from cashflow_rpc.server import create_server
read = lambda name: pathlib.Path(sys.argv[1], name).read_bytes()
server, port = create_server(port=0, certificate=read("server.pem"), private_key=read("server.key"), client_ca=read("ca.pem"))
server.start()
print(port, flush=True)
server.wait_for_termination()
`
	command := exec.CommandContext(ctx, python, "-u", "-c", script, certs)
	command.Env = []string{"PATH=" + filepath.Dir(python), "PYTHONPATH=" + source, "PYTHONDONTWRITEBYTECODE=1"}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = command.Wait() })
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() {
		t.Fatal("ML did not start", scanner.Err())
	}
	port, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
	if err != nil || port < 1 || port > 65535 {
		t.Fatal("invalid ML port", scanner.Text())
	}
	conn, err := grpc.NewClient("127.0.0.1:"+strconv.Itoa(port), grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client, err := New(conn, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	i := financialFixture(t)
	result, err := client.PredictFinancial(ctx, i)
	if err != nil || len(result.Projection.Days) != 30 || result.Projection.Days[29].ReferenceBalance.Minor() != -2700 || len(result.Components) != 30 || result.ProbabilityUnavailableReason != "not_calibrated" {
		t.Fatal(result, err)
	}
	// Reconstructed days preserve provenance through the actual financial model.
	i.Snapshot.History[0].Source = "reconstructed_covered"
	if _, err := client.PredictFinancial(ctx, i); err != nil {
		t.Fatal("reconstructed financial history", err)
	}
	// Covered zero activity with an observed zero EOD is valid; a missing day
	// would not prove inactivity and must never take this path.
	i.Operations = nil
	for n := range i.Snapshot.History {
		i.Snapshot.History[n].Inflow = rub(0)
		i.Snapshot.History[n].Outflow = rub(0)
		i.Snapshot.History[n].ClosingBalance = rub(0)
		i.Snapshot.History[n].Source = forecasting.VerifiedNoOperations
	}
	i.Snapshot.History[89].Source = forecasting.Observed
	i.Snapshot.OpeningBalance = rub(0)
	zero, err := client.PredictFinancial(ctx, i)
	if err != nil || zero.Projection.FirstNegativeDate != nil || len(zero.Components) != 0 || zero.Projection.Days[29].ReferenceBalance.Minor() != 0 || zero.ProbabilityUnavailableReason != "not_calibrated" {
		t.Fatal("covered zero history", zero, err)
	}
}
