# Gerbang project WG-WebUI. Jalankan dari root repo.
SERVER := WG-Server
WEB    := WG-Frontend

.PHONY: check test test-wg e2e build web lab-up lab-down sqlc lint dev-build

# Gerbang Go cepat: format, build, vet, unit test (tanpa kernel).
check:
	cd $(SERVER) && test -z "$$(gofmt -l .)" && go build ./... && go vet ./... && go vet -tags wgintegration ./...
test: check
	cd $(SERVER) && go test -count=1 ./...

# Uji integrasi kernel: SELALU di netns sekali pakai, tidak pernah di host.
# Satu netns sekali pakai per paket: tiap paket mulai dari kernel kosong, tidak mewarisi interface paket lain.
WG_IT_PACKAGES := wg reconcile
test-wg:
	set -e; cd $(SERVER); for pkg in $(WG_IT_PACKAGES); do \
	  go test -c -tags wgintegration -o /tmp/wgpanel-$$pkg.test ./internal/$$pkg/; \
	  ip netns add wgpanel-it; \
	  rc=0; ip netns exec wgpanel-it /tmp/wgpanel-$$pkg.test -test.v -test.count=1 || rc=$$?; \
	  ip netns del wgpanel-it; rm -f /tmp/wgpanel-$$pkg.test; \
	  [ $$rc -eq 0 ] || exit $$rc; \
	done

sqlc:
	cd $(SERVER) && sqlc generate

lint:
	cd $(SERVER) && golangci-lint run ./... && govulncheck ./...

# Frontend dibangun ke dist/ milik paket web agar ikut ter-embed.
web:
	cd $(WEB) && npm ci && npm run check && npm run test && npm run build
	rm -rf $(SERVER)/internal/web/dist && cp -r $(WEB)/dist $(SERVER)/internal/web/dist && touch $(SERVER)/internal/web/dist/.gitkeep

build: web
	cd $(SERVER) && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o wgpanel ./cmd/wgpanel

e2e:
	cd $(WEB) && npx playwright test

lab-up:
	scripts/devlab.sh up
lab-down:
	scripts/devlab.sh down

# Biner dev dipasang di luar /home (lihat komentar wgpanel-dev.service), lalu unit di-restart.
dev-build:
	cd $(SERVER) && CGO_ENABLED=0 go build -o tmp/wgpanel-dev ./cmd/wgpanel
	install -m 755 $(SERVER)/tmp/wgpanel-dev /usr/local/lib/wgpanel-dev/wgpanel
	install -m 755 scripts/devlab.sh /usr/local/lib/wgpanel-dev/devlab.sh
	-systemctl restart wgpanel-dev
