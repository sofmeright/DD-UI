module ddui

go 1.23

require (
	github.com/coreos/go-oidc/v3 v3.11.0
	github.com/go-chi/chi/v5 v5.2.1
	github.com/go-chi/cors v1.2.1
	github.com/gorilla/sessions v1.2.2
	github.com/jackc/pgx/v5 v5.6.0
	golang.org/x/oauth2 v0.22.0
	gopkg.in/yaml.v3 v3.0.1

	// Docker SDK (single module)
	github.com/docker/docker v27.1.2+incompatible
)