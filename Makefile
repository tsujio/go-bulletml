.PHONY: all deploy

all:
	cd simulator && GOOS=js GOARCH=wasm go build -o simulator.wasm github.com/tsujio/go-bulletml/simulator

deploy:
	cp simulator/simulator.wasm demo/
	firebase --project go-bulletml deploy
