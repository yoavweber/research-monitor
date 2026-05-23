package route

import (
	"github.com/yoavweber/research-monitor/backend/internal/application"
	"github.com/yoavweber/research-monitor/backend/internal/http/controller"
	"github.com/yoavweber/research-monitor/backend/internal/http/middleware"
	userpersist "github.com/yoavweber/research-monitor/backend/internal/infrastructure/persistence/user"
)

// AuthRouter wires the three auth endpoints. POST /auth/login mounts on the
// root group so unauthenticated callers can exchange credentials for a JWT;
// GET /auth/session and POST /auth/change-password sit behind JWTAuth on an
// /auth subgroup so they require a valid bearer token. The validator is used
// only by the middleware — the controller depends solely on the use-case.
func AuthRouter(d Deps) {
	repo := userpersist.NewRepository(d.DB)
	uc := application.NewUserUseCase(repo, d.Hasher, d.Signer, d.Clock, d.Logger)
	ctrl := controller.NewAuthController(uc)

	d.RootGroup.POST("/auth/login", ctrl.Login)

	authed := d.RootGroup.Group("/auth", middleware.JWTAuth(d.Validator))
	authed.GET("/session", ctrl.Session)
	authed.POST("/change-password", ctrl.ChangePassword)
}
