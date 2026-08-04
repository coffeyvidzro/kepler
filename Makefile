COMPOSE := docker compose --env-file .env -f deploy/compose.yaml

.PHONY: help
help:
	@echo "Dugble deployment commands:"
	@echo "  make up       Build and start the stack"
	@echo "  make down     Stop the stack"
	@echo "  make deploy   Pull and deploy the latest main branch"
	@echo "  make logs     Follow logs"
	@echo "  make ps       Show service status"
	@echo "  make migrate  Run database migrations"
	@echo "  make restart  Restart application services"

.PHONY: up
up:
	$(COMPOSE) up -d --build

.PHONY: down
down:
	$(COMPOSE) down

.PHONY: deploy
deploy:
	git pull --ff-only origin main
	$(COMPOSE) up -d --build
	$(COMPOSE) ps

.PHONY: logs
logs:
	$(COMPOSE) logs -f --tail=200

.PHONY: ps
ps:
	$(COMPOSE) ps

.PHONY: migrate
migrate:
	$(COMPOSE) run --rm migrate

.PHONY: restart
restart:
	$(COMPOSE) restart server worker backoffice caddy
