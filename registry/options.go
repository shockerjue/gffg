package registry

// Service location information
type location struct {
	region string
	zone   string
	campus string
}

// Service node information
type nodeopts struct {
	group    string
	name     string
	version  string
	token    string
	location location
}

type NodeOption func(*nodeopts)

func Group(group string) NodeOption {
	return func(c *nodeopts) {
		c.group = group
	}
}

func Token(token string) NodeOption {
	return func(c *nodeopts) {
		c.token = token
	}
}

func Version(version string) NodeOption {
	return func(c *nodeopts) {
		c.version = version
	}
}

func Name(name string) NodeOption {
	return func(c *nodeopts) {
		c.name = name
	}
}

func Region(region string) NodeOption {
	return func(c *nodeopts) {
		c.location.region = region
	}
}

func Zone(zone string) NodeOption {
	return func(c *nodeopts) {
		c.location.zone = zone
	}
}

func Campus(campus string) NodeOption {
	return func(c *nodeopts) {
		c.location.campus = campus
	}
}

type node struct {
	opts nodeopts
}

func Node(opts ...NodeOption) *node {
	n := &node{}
	for _, o := range opts {
		o(&n.opts)
	}

	return n
}
