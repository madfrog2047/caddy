package errors

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mholt/caddy/caddyhttp/httpserver"
)

func TestErrors(t *testing.T) {
	// create a temporary page
	path := filepath.Join(os.TempDir(), "errors_test.html")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	const content = "This is a error page"
	_, err = f.WriteString(content)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	buf := bytes.Buffer{}
	em := ErrorHandler{
		ErrorPages: map[int]string{
			http.StatusNotFound:  path,
			http.StatusForbidden: "not_exist_file",
		},
		Log: log.New(&buf, "", 0),
	}
	_, notExistErr := os.Open("not_exist_file")

	testErr := errors.New("test error")
	tests := []struct {
		next         httpserver.Handler
		expectedCode int
		expectedBody string
		expectedLog  string
		expectedErr  error
	}{
		{
			next:         genErrorHandler(http.StatusOK, nil, "normal"),
			expectedCode: http.StatusOK,
			expectedBody: "normal",
			expectedLog:  "",
			expectedErr:  nil,
		},
		{
			next:         genErrorHandler(http.StatusMovedPermanently, testErr, ""),
			expectedCode: http.StatusMovedPermanently,
			expectedBody: "",
			expectedLog:  fmt.Sprintf("[ERROR %d %s] %v\n", http.StatusMovedPermanently, "/", testErr),
			expectedErr:  testErr,
		},
		{
			next:         genErrorHandler(http.StatusBadRequest, nil, ""),
			expectedCode: 0,
			expectedBody: fmt.Sprintf("%d %s\n", http.StatusBadRequest,
				http.StatusText(http.StatusBadRequest)),
			expectedLog: "",
			expectedErr: nil,
		},
		{
			next:         genErrorHandler(http.StatusNotFound, nil, ""),
			expectedCode: 0,
			expectedBody: content,
			expectedLog:  "",
			expectedErr:  nil,
		},
		{
			next:         genErrorHandler(http.StatusForbidden, nil, ""),
			expectedCode: 0,
			expectedBody: fmt.Sprintf("%d %s\n", http.StatusForbidden,
				http.StatusText(http.StatusForbidden)),
			expectedLog: fmt.Sprintf("[NOTICE %d /] could not load error page: %v\n",
				http.StatusForbidden, notExistErr),
			expectedErr: nil,
		},
	}

	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, test := range tests {
		em.Next = test.next
		buf.Reset()
		rec := httptest.NewRecorder()
		code, err := em.ServeHTTP(rec, req)

		if err != test.expectedErr {
			t.Errorf("Test %d: Expected error %v, but got %v",
				i, test.expectedErr, err)
		}
		if code != test.expectedCode {
			t.Errorf("Test %d: Expected status code %d, but got %d",
				i, test.expectedCode, code)
		}
		if body := rec.Body.String(); body != test.expectedBody {
			t.Errorf("Test %d: Expected body %q, but got %q",
				i, test.expectedBody, body)
		}
		if log := buf.String(); !strings.Contains(log, test.expectedLog) {
			t.Errorf("Test %d: Expected log %q, but got %q",
				i, test.expectedLog, log)
		}
	}
}

func TestVisibleErrorWithPanic(t *testing.T) {
	const panicMsg = "I'm a panic"
	eh := ErrorHandler{
		ErrorPages: make(map[int]string),
		Debug:      true,
		Next: httpserver.HandlerFunc(func(w http.ResponseWriter, r *http.Request) (int, error) {
			panic(panicMsg)
		}),
	}

	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()

	code, err := eh.ServeHTTP(rec, req)

	if code != 0 {
		t.Errorf("Expected error handler to return 0 (it should write to response), got status %d", code)
	}
	if err != nil {
		t.Errorf("Expected error handler to return nil error (it should panic!), but got '%v'", err)
	}

	body := rec.Body.String()

	if !strings.Contains(body, "[PANIC /] caddyhttp/errors/errors_test.go") {
		t.Errorf("Expected response body to contain error log line, but it didn't:\n%s", body)
	}
	if !strings.Contains(body, panicMsg) {
		t.Errorf("Expected response body to contain panic message, but it didn't:\n%s", body)
	}
	if len(body) < 500 {
		t.Errorf("Expected response body to contain stack trace, but it was too short: len=%d", len(body))
	}
}

func genErrorHandler(status int, err error, body string) httpserver.Handler {
	return httpserver.HandlerFunc(func(w http.ResponseWriter, r *http.Request) (int, error) {
		if len(body) > 0 {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			fmt.Fprint(w, body)
		}
		return status, err
	})
}
