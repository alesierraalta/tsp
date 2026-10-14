package publish

import "errors"

type Step struct {
	Apply func() error
	Undo  func() error
}

func Publish(steps []Step) error {
	completed := 0
	for _, step := range steps {
		if err := step.Apply(); err != nil {
			failure := err
			for i := completed - 1; i > 0; i-- {
				if cleanup := steps[i].Undo(); cleanup != nil {
					failure = errors.Join(failure, cleanup)
				}
			}
			return failure
		}
		completed++
	}
	return nil
}
