// src/api/go.mod
module dd-ui

go 1.25

require (
	github.com/coreos/go-oidc/v3 v3.15.0
	github.com/go-chi/chi/v5 v5.2.3
	github.com/go-chi/cors v1.2.2
	github.com/alexedwards/scs/v2 v2.8.0
	github.com/jackc/pgx/v5 v5.7.2
	golang.org/x/crypto v0.42.0
	golang.org/x/oauth2 v0.31.0
	gopkg.in/yaml.v3 v3.0.1

	// Docker SDK (single module)
	github.com/docker/docker v27.5.1+incompatible
)