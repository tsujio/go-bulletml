.PHONY: all deploy serve

all:
	cd simulator && GOOS=js GOARCH=wasm go build -o simulator.wasm github.com/tsujio/go-bulletml/simulator

deploy:
	cp simulator/simulator.wasm demo/
	firebase --project go-bulletml deploy

serve:
	cp simulator/simulator.wasm demo/
	firebase serve --port 8000
