package metrics

type MetricOption func(*option)
type option struct {
	serveName string
}

func ServerName(serveName string) MetricOption {
	return func(c *option) {
		c.serveName = serveName
	}
}
