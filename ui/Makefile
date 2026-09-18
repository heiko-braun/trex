.PHONY: dev run build preview install

## install: install npm dependencies
install:
	npm install

## dev: run the Vite dev server (proxies /definitions and /api to VITE_API_BASE_URL, default http://localhost:8080)
dev: node_modules
	npm run dev

## run: alias for dev, matching the root Makefile's `make run` for the backend
run: dev

## build: type-check and build the production bundle
build: node_modules
	npm run build

## preview: preview the production build locally
preview: build
	npm run preview

node_modules:
	npm install

.DEFAULT_GOAL := dev
