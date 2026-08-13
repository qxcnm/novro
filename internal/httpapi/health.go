package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type databasePinger interface {
	/**
	 * PingContext 声明该接口方法需要提供的业务能力。
	 * @param arg1 类型为 context.Context 的接口输入参数。
	 * @author Gao Hongshun
	 * @date 2026-08-13
	 */
	PingContext(context.Context) error
}

type healthHandler struct {
	db databasePinger
}

/**
 * NewHealthHandler 用于创建并返回所需的对象或记录。
 * @param db 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func NewHealthHandler(db databasePinger) http.Handler {
	h := healthHandler{db: db}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.live)
	mux.HandleFunc("GET /readyz", h.ready)
	return mux
}

/**
 * live 封装该名称对应的业务处理逻辑。
 * @param w HTTP 响应写入器。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (h healthHandler) live(w http.ResponseWriter, _ *http.Request) {
	writeHealth(w, http.StatusOK, "ok")
}

/**
 * ready 封装该名称对应的业务处理逻辑。
 * @param w HTTP 响应写入器。
 * @param r 当前 HTTP 请求。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func (h healthHandler) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if h.db == nil || h.db.PingContext(ctx) != nil {
		writeHealth(w, http.StatusServiceUnavailable, "not_ready")
		return
	}
	writeHealth(w, http.StatusOK, "ok")
}

/**
 * writeHealth 封装该名称对应的业务处理逻辑。
 * @param w HTTP 响应写入器。
 * @param status 用于标识或筛选目标的文本值。
 * @param state 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func writeHealth(w http.ResponseWriter, status int, state string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": state})
}
