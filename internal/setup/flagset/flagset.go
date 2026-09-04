package flagset

import "github.com/spf13/pflag"

type Wrapper struct {
	*pflag.FlagSet
	sets []*pflag.FlagSet
}

func NewWrapper(set *pflag.FlagSet) *Wrapper {
	return &Wrapper{
		FlagSet: set,
		//sets:    []*pflag.FlagSet{set},
	}
}

func (f *Wrapper) AddFlagSet(set *pflag.FlagSet) {
	f.FlagSet.AddFlagSet(set)
	f.sets = append(f.sets, set)
}

func (f *Wrapper) FlagSets() []*pflag.FlagSet {
	return f.sets
}
