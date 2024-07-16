package controller

type controller struct {
}

func newController() *controller {
	v := &controller{}

	return v
}

func Controller() *controller {
	return newController()
}
