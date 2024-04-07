package djs

type Options struct {
	withGlobalHeap bool
	moduleHome string
}

type Option func(*Options)

func WithoutGlobalHeap() Option {
	return func(options *Options) {
		options.withGlobalHeap = false
	}
}

func WithGlobalHeap() Option {
	return func(options *Options) {
		options.withGlobalHeap = true
	}
}

func WithModuleHome(moduleHome string) Option {
	return func(options *Options) {
		options.moduleHome = moduleHome
	}
}

func getOptions(options ...Option) *Options {
	var option Options
	for _, o := range options {
		o(&option)
	}

	return &option
}

