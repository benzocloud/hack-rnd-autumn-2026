//go:build integration && mlintegration

package app

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
	"google.golang.org/protobuf/proto"

	mlClient "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/client/ml"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/config"
	pb "github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/gen/core/v1"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/core/internal/transport/mtls"
)

func pythonFinancialModel(t *testing.T, f *manualBalanceFixture) financialPredictor {
	t.Helper()
	python, source := os.Getenv("CORE_ML_TEST_PYTHON"), os.Getenv("CORE_ML_TEST_SOURCE")
	if python == "" || source == "" {
		t.Skip("set isolated Python and generated ML source path")
	}
	files := f.ca.Files(t, mtls.ML)
	ctx, cancel := context.WithCancel(t.Context())
	script := `import pathlib, sys
from cashflow_rpc.server import create_server
read = lambda name: pathlib.Path(name).read_bytes()
server, port = create_server(port=0, certificate=read(sys.argv[1]), private_key=read(sys.argv[2]), client_ca=read(sys.argv[3]), allowed_peers=("spiffe://saldo.local/core",))
server.start()
print(port, flush=True)
server.wait_for_termination()
`
	command := exec.CommandContext(ctx, python, "-u", "-c", script, files.Certificate, files.Key, files.CA)
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
		t.Fatal("ML process did not start", scanner.Err())
	}
	port, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
	if err != nil || port < 1 || port > 65535 {
		t.Fatal(scanner.Text(), err)
	}
	tlsConfig, err := mtls.Client(f.ca.Files(t, mtls.Core), "saldo-core", mtls.Core, mtls.ML)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := grpc.NewClient("127.0.0.1:"+strconv.Itoa(port), grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)), grpc.WithDisableRetry())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	model, err := mlClient.New(conn, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return model
}

func TestForecastRPCPostgresWorkerWithRealPythonML(t *testing.T) {
	f := newManualBalanceFixture(t)
	cutoff, _ := seedForecastHistory(t, f, true)
	model := pythonFinancialModel(t, f)
	jobs := newForecastJobs(f.app.db, config.Postgres{QueryTimeout: 5 * time.Second}, model)
	client := forecastRPC(t, f, jobs, mtls.API)
	request := createForecastRequest(f, cutoff)
	accepted, err := client.CreateForecast(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- jobs.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(10 * time.Second):
			t.Error("worker did not terminate")
		}
	})
	get := &pb.GetForecastRequest{Context: request.Context.Browser, Scope: request.Scope, ForecastId: accepted.Accepted.Id}
	deadline := time.Now().Add(10 * time.Second)
	var ready *pb.GetForecastResponse
	for time.Now().Before(deadline) {
		ready, err = client.GetForecast(t.Context(), get)
		if err != nil {
			t.Fatal(err)
		}
		if ready.Forecast.GetSucceeded() != nil || ready.Forecast.GetFailed() != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if ready == nil || ready.Forecast.GetSucceeded() == nil {
		t.Fatal("real worker did not succeed", ready)
	}
	result := ready.Forecast.GetSucceeded().Result
	if len(result.DailyPoints) != 30 || result.Metrics.ClosingBalance.AmountMinor != -2700 || result.Metrics.MaxShortfall.AmountMinor != 2700 || result.Metrics.GapProbability.GetUnavailableReason() != "not_calibrated" || result.ScenarioCapabilities.DelayReceipt || ready.Forecast.Input.ModelContext.ModelVersion != "mean_90_components_v1" {
		t.Fatal(ready)
	}
	second, cfg := testApplication(t)
	again := newForecastJobs(second.db, cfg.Postgres, model)
	reconnected := forecastRPC(t, f, again, mtls.API)
	stored, err := reconnected.GetForecast(t.Context(), get)
	if err != nil || !proto.Equal(stored, ready) {
		t.Fatal("persisted result changed after reconnect", stored, err)
	}
	listed, err := reconnected.ListForecasts(t.Context(), &pb.ListForecastsRequest{Context: request.Context.Browser, Scope: request.Scope})
	if err != nil || len(listed.Items) != 1 || listed.Items[0].Status != pb.ForecastStatus_FORECAST_STATUS_SUCCEEDED {
		t.Fatal(listed, err)
	}
}
