//go:build integration

package integration_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/app"
	"github.com/benzocloud/hack-rnd-autumn-2026/services/auth/internal/repository/postgres"
)

func TestRealDatabaseReadinessAndServiceRestart(t *testing.T) {
	cfg,db:=scratch(t)
	if err:=postgres.Migrate(t.Context(),cfg);err!=nil {t.Fatal(err)}
	client:=&http.Client{Timeout:time.Second}
	defer client.CloseIdleConnections()
	for range 2 {
		listener,err:=net.Listen("tcp","127.0.0.1:0")
		if err!=nil {t.Fatal(err)}
		cfg.HTTPAddress=listener.Addr().String()
		if err:=listener.Close();err!=nil {t.Fatal(err)}
		ctx,cancel:=context.WithCancel(t.Context())
		done:=make(chan error,1)
		go func(){done<-app.Run(ctx,cfg,slog.New(slog.NewTextHandler(io.Discard,nil)))}()
		base:="http://"+cfg.HTTPAddress
		deadline:=time.Now().Add(5*time.Second)
		ready:=false
		for time.Now().Before(deadline) {
			response,err:=client.Get(base+"/health/ready")
			if err==nil {_=response.Body.Close();if response.StatusCode==200 {ready=true;break}}
			select{case err:=<-done:cancel();t.Fatalf("server exited before ready: %v",err);default:}
			time.Sleep(10*time.Millisecond)
		}
		if !ready {cancel();<-done;t.Fatal("real service never became ready")}
		// Liveness survives schema incompatibility; readiness must fail closed.
		if _,err:=db.Exec(t.Context(),"UPDATE goose_db_version SET version_id=99 WHERE version_id=1");err!=nil {cancel();<-done;t.Fatal(err)}
		for _,tc:=range []struct{path string;status int}{{"/health/ready",503},{"/health/live",200},{"/api/v1/auth/register",404}} {
			response,err:=client.Get(base+tc.path)
			if err!=nil {cancel();<-done;t.Fatal(err)}
			_=response.Body.Close()
			if response.StatusCode!=tc.status {cancel();<-done;t.Fatalf("%s: %d",tc.path,response.StatusCode)}
		}
		if _,err:=db.Exec(t.Context(),"UPDATE goose_db_version SET version_id=1 WHERE version_id=99");err!=nil {cancel();<-done;t.Fatal(err)}
		cancel()
		select{case err:=<-done:if err!=nil {t.Fatal(err)};case <-time.After(6*time.Second):t.Fatal("real server shutdown hung")}
		connection,err:=net.DialTimeout("tcp",cfg.HTTPAddress,100*time.Millisecond)
		if err==nil {_=connection.Close();t.Fatal("real listener leaked after shutdown")}
	}
}
