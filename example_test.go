package ecs_test

import (
	"fmt"

	"github.com/trancecode/ecs"
)

type Position struct{ X, Y float64 }
type Velocity struct{ DX, DY float64 }

func Example() {
	w := ecs.NewWorld()
	positions := ecs.Components[Position](w)
	moving := ecs.Components2[Position, Velocity](w)

	e := w.NewEntity()
	positions.Add(e, Position{X: 0})
	moving.Add(e, Position{X: 0}, Velocity{DX: 3})

	for _, tup := range moving.All() {
		p, v := tup.Values()
		p.X += v.DX
	}

	p, _ := positions.Get(e)
	fmt.Printf("%.0f\n", p.X)
	// Output: 3
}
