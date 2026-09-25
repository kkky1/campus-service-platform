package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// 等价 WebExceptionAdvice：panic → 200 + {"success":false,"errorMsg":"服务器异常"}。
func TestRecoveryContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Recovery(slog.Default()))
	r.GET("/boom", func(c *gin.Context) {
		panic("something wrong")
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/boom", nil))
	if w.Code != 200 {
		t.Fatalf("panic 响应码 = %d, want 200", w.Code)
	}
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m["success"] != false || m["errorMsg"] != "服务器异常" {
		t.Fatalf("panic 响应不对: %v", m)
	}
	if len(m) != 2 {
		t.Fatalf("panic 响应不应含堆栈等多余键: %v", m)
	}
}
