//go:build js && wasm

package main

import (
	"bytes"
	"syscall/js"
)

func init() {
	js.Global().Set("setBulletML", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 1 {
			return nil
		}

		source := args[0].String()

		game.samples = nil

		game.appendSample("", bytes.NewReader([]byte(source)))

		game.initializeRunner()

		return nil
	}))
}
