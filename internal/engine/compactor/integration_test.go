package compactor

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// --- Realistic test inputs ---

var jsonStructuredLog = `{"level":"error","msg":"connection timeout to payment service","trace_id":"a1b2c3d4e5f6","span_id":"1234abcd","request_id":"req-99887766","service":"checkout","host":"api-east-1","latency_ms":30000,"status_code":504,"path":"/api/v2/payments/charge","method":"POST","user_id":"usr_4821","correlation_id":"corr-xyz-789","dd.trace_id":"8877665544","dd.span_id":"1122334455"}`

var javaStackTrace = `java.lang.NullPointerException: Cannot invoke method on null reference
	at com.example.payments.PaymentService.processCharge(PaymentService.java:142)
	at com.example.payments.PaymentService.charge(PaymentService.java:98)
	at com.example.api.PaymentController.handleCharge(PaymentController.java:67)
	at com.example.api.PaymentController.post(PaymentController.java:42)
	at org.springframework.web.servlet.FrameworkServlet.service(FrameworkServlet.java:897)
	at javax.servlet.http.HttpServlet.service(HttpServlet.java:750)
	at org.apache.catalina.core.ApplicationFilterChain.doFilter(ApplicationFilterChain.java:231)
	at org.apache.catalina.core.ApplicationFilterChain.internalDoFilter(ApplicationFilterChain.java:178)
	at org.springframework.web.filter.RequestContextFilter.doFilterInternal(RequestContextFilter.java:100)
	at org.springframework.web.filter.OncePerRequestFilter.doFilter(OncePerRequestFilter.java:107)
	at org.apache.catalina.core.ApplicationFilterChain.doFilter(ApplicationFilterChain.java:231)
	at org.springframework.web.filter.CharacterEncodingFilter.doFilterInternal(CharacterEncodingFilter.java:201)
	at org.springframework.web.filter.OncePerRequestFilter.doFilter(OncePerRequestFilter.java:107)
	at org.apache.catalina.core.ApplicationFilterChain.doFilter(ApplicationFilterChain.java:231)
	at org.apache.catalina.core.StandardWrapperValve.invoke(StandardWrapperValve.java:213)
	at org.apache.catalina.core.StandardContextValve.invoke(StandardContextValve.java:175)
	at org.apache.catalina.authenticator.AuthenticatorBase.invoke(AuthenticatorBase.java:525)
	at org.apache.catalina.core.StandardHostValve.invoke(StandardHostValve.java:112)
	at org.apache.catalina.valves.ErrorReportValve.invoke(ErrorReportValve.java:83)
	at org.apache.catalina.core.StandardEngineValve.invoke(StandardEngineValve.java:78)
	at org.apache.catalina.connector.CoyoteAdapter.service(CoyoteAdapter.java:423)
	at org.apache.coyote.http11.Http11Processor.service(Http11Processor.java:374)
	at org.apache.coyote.AbstractProcessorLight.process(AbstractProcessorLight.java:65)
	at org.apache.coyote.AbstractProtocol$ConnectionHandler.process(AbstractProtocol.java:868)
	at org.apache.tomcat.util.net.NioEndpoint$SocketProcessor.doRun(NioEndpoint.java:1590)
	at org.apache.tomcat.util.net.SocketProcessorBase.run(SocketProcessorBase.java:49)
	at java.util.concurrent.ThreadPoolExecutor.runWorker(ThreadPoolExecutor.java:1149)
	at java.util.concurrent.ThreadPoolExecutor$Worker.run(ThreadPoolExecutor.java:624)
	at org.apache.tomcat.util.threads.TaskThread$WrappingRunnable.run(TaskThread.java:61)
	at java.lang.Thread.run(Thread.java:748)`

var goPanicDump = `goroutine 1 [running]:
main.processRequest(0xc0000b4000, 0x1a4)
	/app/cmd/server/main.go:142 +0x2a5
net/http.(*ServeMux).ServeHTTP(0xc0000a8000, {0x7f4c20, 0xc0000b2000}, 0xc0000b4000)
	/usr/local/go/src/net/http/server.go:2487 +0x149
net/http.serverHandler.ServeHTTP({0xc000098060}, {0x7f4c20, 0xc0000b2000}, 0xc0000b4000)
	/usr/local/go/src/net/http/server.go:2908 +0x43f
net/http.(*conn).serve(0xc0000b0000, {0x7f5460, 0xc000096480})
	/usr/local/go/src/net/http/server.go:1989 +0x1297
net/http.(*Server).Serve.func3()
	/usr/local/go/src/net/http/server.go:3101 +0x4f
runtime.goexit()
	/usr/local/go/src/runtime/asm_amd64.s:1571 +0x1
goroutine 2 [running]:
database/sql.(*DB).connectionOpener(0xc00009e000, {0x7f5460, 0xc000096480})
	/usr/local/go/src/database/sql/sql.go:1189 +0x85
goroutine 3 [running]:
database/sql.(*DB).connectionResetter(0xc00009e000, {0x7f5460, 0xc000096480})
	/usr/local/go/src/database/sql/sql.go:1207 +0x85
goroutine 4 [running]:
net/http.(*connReader).backgroundRead(0xc0000b0070)
	/usr/local/go/src/net/http/server.go:678 +0x3e`

var plainTextError = `ERROR [2026-02-19 12:00:00.123] UserService — connection refused to database
host=db-primary port=5432 retries=3 last_error="dial tcp 10.0.1.50:5432: connect: connection refused"
service=user-api region=us-east-1 deployment=v2.4.1`

var shortRequestLog = `2026-02-19T12:00:00Z INFO GET /api/v2/health 200 2ms`

// --- Integration tests ---

func TestIntegrationMinimalStackTrace(t *testing.T) {
	cmp := New(Minimal)
	compacted, summary := cmp.Compact(javaStackTrace, "ERROR")

	// Should be valid UTF-8.
	if !utf8.ValidString(compacted) {
		t.Fatal("compacted is not valid UTF-8")
	}

	// Stack trace should be truncated.
	if !strings.Contains(compacted, "frames omitted") {
		t.Fatal("expected stack trace truncation at Minimal")
	}

	// Summary should be first line.
	if !strings.HasPrefix(summary, "java.lang.NullPointerException") {
		t.Fatalf("expected NullPointerException in summary, got %q", summary)
	}

	// Token reduction.
	tokensBefore := EstimateTokens(javaStackTrace)
	tokensAfter := EstimateTokens(compacted)
	reduction := float64(tokensBefore-tokensAfter) / float64(tokensBefore) * 100
	if reduction < 60 {
		t.Fatalf("expected >60%% token reduction, got %.1f%% (before=%d, after=%d)",
			reduction, tokensBefore, tokensAfter)
	}
}

func TestIntegrationStandardStructuredLog(t *testing.T) {
	cmp := New(Standard)
	compacted, _ := cmp.Compact(jsonStructuredLog, "ERROR")

	// trace_id, span_id, request_id, correlation_id, dd.trace_id, dd.span_id should be stripped.
	for _, field := range []string{"trace_id", "span_id", "request_id", "correlation_id", "dd.trace_id", "dd.span_id"} {
		if strings.Contains(compacted, `"`+field+`"`) {
			t.Fatalf("expected %s to be stripped at Standard, found in: %s", field, compacted)
		}
	}

	// Core fields should be preserved.
	for _, field := range []string{"level", "msg", "service", "status_code"} {
		if !strings.Contains(compacted, field) {
			t.Fatalf("expected %s to be preserved, not found in: %s", field, compacted)
		}
	}

	if !utf8.ValidString(compacted) {
		t.Fatal("compacted is not valid UTF-8")
	}
}

func TestIntegrationFullPreservesEverything(t *testing.T) {
	cmp := New(Full)

	// JSON log.
	compacted, _ := cmp.Compact(jsonStructuredLog, "ERROR")
	if compacted != jsonStructuredLog {
		t.Fatal("Full should preserve JSON log unchanged")
	}

	// Stack trace.
	compacted, _ = cmp.Compact(javaStackTrace, "ERROR")
	if compacted != javaStackTrace {
		t.Fatal("Full should preserve stack trace unchanged")
	}
}

func TestIntegrationMultibyteUTF8(t *testing.T) {
	cjkLog := `{"level":"error","msg":"データベース接続タイムアウト: ホスト=db-primary ポート=5432 リトライ回数=3","trace_id":"abc123","service":"ユーザーサービス"}`

	for _, v := range []Verbosity{Minimal, Standard, Full} {
		cmp := New(v)
		compacted, summary := cmp.Compact(cjkLog, "ERROR")

		if !utf8.ValidString(compacted) {
			t.Fatalf("compacted is not valid UTF-8 at verbosity %d", v)
		}
		if !utf8.ValidString(summary) {
			t.Fatalf("summary is not valid UTF-8 at verbosity %d", v)
		}
	}
}

func TestIntegrationGoPanicMinimal(t *testing.T) {
	cmp := New(Minimal)
	compacted, summary := cmp.Compact(goPanicDump, "ERROR")

	if !utf8.ValidString(compacted) {
		t.Fatal("compacted is not valid UTF-8")
	}

	// Should truncate Go frames.
	if !strings.Contains(compacted, "frames omitted") {
		t.Fatal("expected Go panic frame truncation at Minimal")
	}

	// Summary should be first line.
	if !strings.Contains(summary, "goroutine 1") {
		t.Fatalf("expected goroutine reference in summary, got %q", summary)
	}

	tokensBefore := EstimateTokens(goPanicDump)
	tokensAfter := EstimateTokens(compacted)
	if tokensAfter >= tokensBefore {
		t.Fatalf("expected token reduction, got before=%d after=%d", tokensBefore, tokensAfter)
	}
}

func TestIntegrationPlainTextError(t *testing.T) {
	cmp := New(Minimal)
	compacted, summary := cmp.Compact(plainTextError, "ERROR")

	if !utf8.ValidString(compacted) {
		t.Fatal("not valid UTF-8")
	}
	// Plain text — no stripping should occur.
	if !strings.Contains(summary, "UserService") {
		t.Fatalf("expected UserService in summary, got %q", summary)
	}
	// Should be truncated at 200 runes.
	if utf8.RuneCountInString(compacted) > 203 { // 200 + "..."
		t.Fatalf("expected truncation at 200 runes, got %d", utf8.RuneCountInString(compacted))
	}
}

func TestIntegrationShortLog(t *testing.T) {
	cmp := New(Minimal)
	compacted, summary := cmp.Compact(shortRequestLog, "REQUEST")

	// Short enough — should not be truncated.
	if compacted != shortRequestLog {
		t.Fatalf("expected unchanged short log, got %q", compacted)
	}
	if summary != shortRequestLog {
		t.Fatalf("expected summary = input for short log, got %q", summary)
	}
}
