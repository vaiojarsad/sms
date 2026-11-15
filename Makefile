IMAGE_NAME ?= sms
REGISTRY ?= 19741976.xyz
PORT ?= 8080
HOST_PORT ?= 8080
DBUS_SOCKET ?= /run/dbus/system_bus_socket
VERSION = $(shell sh scripts/version.sh)
COMMIT = $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)
GOEXE = $(shell go env GOEXE)

.PHONY: build build-docker run-docker publish

build:
	mkdir -p bin
	go build -trimpath -ldflags="-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)" -o bin/sms$(GOEXE) .

build-docker:
	docker build --build-arg VERSION="$(VERSION)" --build-arg COMMIT="$(COMMIT)" -t $(IMAGE_NAME) .

# Publica una release desde un tag vMAJOR.MINOR.PATCH en un checkout limpio.
publish:
	@set -eu; \
	status=$$(git status --porcelain --untracked-files=all); \
	if [ -n "$$status" ]; then \
		echo "Error: publish requiere un checkout limpio, sin cambios ni archivos sin seguimiento." >&2; \
		exit 1; \
	fi; \
	tag=$$(git describe --tags --exact-match --match 'v[0-9]*' HEAD) || { \
		echo "Error: HEAD debe tener un tag de release vMAJOR.MINOR.PATCH." >&2; \
		exit 1; \
	}; \
	if ! printf '%s\n' "$$tag" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$$'; then \
		echo "Error: el tag debe tener el formato vMAJOR.MINOR.PATCH (por ejemplo, v0.1.0)." >&2; \
		exit 1; \
	fi; \
	commit=$$(git rev-parse --short=12 HEAD); \
	image="$(REGISTRY)/$(IMAGE_NAME):$${tag#v}"; \
	docker build --build-arg VERSION="$${tag#v}" --build-arg COMMIT="$$commit" -t "$$image" .; \
	docker push "$$image"

run-docker:
	docker run --rm -e PORT=$(PORT) -p $(HOST_PORT):$(PORT) \
		-v $(DBUS_SOCKET):/run/dbus/system_bus_socket:ro \
		$(IMAGE_NAME)
