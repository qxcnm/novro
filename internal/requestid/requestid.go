package requestid

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

const Header = "X-Novro-Request-ID"

type contextKey struct{}

/**
 * New 用于创建并返回所需的对象或记录。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func New() uuid.UUID {
	return uuid.New()
}

/**
 * WithContext 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @param id 目标资源的唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func WithContext(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

/**
 * FromContext 封装该名称对应的业务处理逻辑。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func FromContext(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(contextKey{}).(uuid.UUID)
	return id
}

/**
 * Ensure 用于校验输入或运行状态是否满足要求。
 * @param ctx 请求上下文，用于传递取消信号、截止时间和请求级数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func Ensure(ctx context.Context) (context.Context, uuid.UUID) {
	if id := FromContext(ctx); id != uuid.Nil {
		return ctx, id
	}
	id := New()
	return WithContext(ctx, id), id
}

/**
 * Middleware 封装该名称对应的业务处理逻辑。
 * @param next 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, id := Ensure(r.Context())
		w.Header().Set(Header, id.String())
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

/**
 * ResponseID 封装该名称对应的业务处理逻辑。
 * @param w HTTP 响应写入器。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
func ResponseID(w http.ResponseWriter) uuid.UUID {
	id, err := uuid.Parse(w.Header().Get(Header))
	if err != nil {
		return uuid.Nil
	}
	return id
}
