package main

import (
	"github.com/cng1985/edugo/internal/app"
	"go.uber.org/fx"
)

func main() {
	fx.New(app.Options()...).Run()
}
