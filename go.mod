module kanban

go 1.24.1

toolchain go1.24.4

require (
	github.com/asccclass/sherryserver v1.1.0
	github.com/gorilla/websocket v1.5.3
	github.com/joho/godotenv v1.5.1
	github.com/mattn/go-sqlite3 v1.14.22
)

require (
	github.com/asccclass/sherrytime v0.0.3 // indirect
	github.com/coder/websocket v1.8.13 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/securecookie v1.1.2 // indirect
	github.com/gorilla/sessions v1.4.0 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
)

replace go.uber.org/zap => ./vendor_patches/go.uber.org/zap

replace go.uber.org/multierr => ./vendor_patches/go.uber.org/multierr

replace golang.org/x/sys => ./vendor_patches/golang.org/x/sys

replace go.uber.org/goleak => ./vendor_patches/go.uber.org/goleak

replace gopkg.in/yaml.v3 => ./vendor_patches/gopkg.in/yaml.v3
