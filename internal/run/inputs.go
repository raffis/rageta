package run

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/raffis/rageta/internal/pipeline"
	"github.com/raffis/rageta/internal/setup/flagset"
	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
	"github.com/spf13/pflag"
)

type InputsOptions struct {
	Args []string
}

func (s InputsOptions) Build() Step {
	return &Inputs{opts: s}
}

func (s *InputsOptions) BindFlags(flags flagset.Interface) {
	flags.StringArrayVarP(&s.Args, "input", "i", s.Args, "Pass inputs to the pipeline.")
}

type Inputs struct {
	opts InputsOptions
}

type InputsContext struct {
	Args map[string]v1beta1.ParamValue
}

func (s *Inputs) Run(rc *RunContext, next Next) error {
	flagSet := pflag.NewFlagSet("inputs", pflag.ContinueOnError)
	pipeline.Flags(flagSet, rc.Provider.Pipeline.Inputs)

	if flagStart := slices.Index(os.Args, "--"); flagStart != -1 {
		if err := flagSet.Parse(os.Args[flagStart+1:]); err != nil {
			return err
		}
	}

	inputs, err := s.parseInputsFromFlags(rc.Provider.Pipeline.Inputs, s.opts.Args, flagSet)
	if err != nil {
		return err
	}

	rc.Inputs.Args = inputs
	return next(rc)
}

func (s *Inputs) parseInputsFromFlags(params []v1beta1.InputParam, inputs []string, flagSet *pflag.FlagSet) (map[string]v1beta1.ParamValue, error) {
	result := make(map[string]v1beta1.ParamValue)
	steps := make(map[string][]string)

	for _, v := range inputs {
		flag := strings.SplitN(v, "=", 2)
		if len(flag) != 2 {
			return result, errors.New("expected input key=value")
		}
		steps[flag[0]] = append(steps[flag[0]], flag[1])
	}

	for _, v := range params {
		if input, ok := steps[v.Name]; ok {
			x := result[v.Name]
			if len(input) == 1 {
				if err := x.UnmarshalJSON([]byte(input[0])); err != nil {
					return result, fmt.Errorf("failed to decode input: %w", err)
				}
				result[v.Name] = x
				continue
			}
			x.Type = v1beta1.ParamTypeArray
			x.ArrayVal = input
			result[v.Name] = x
		}
	}

	flagSet.Visit(func(f *pflag.Flag) {
		switch f.Value.Type() {
		case "string":
			val, _ := flagSet.GetString(f.Name)
			result[f.Name] = v1beta1.ParamValue{Type: v1beta1.ParamTypeString, StringVal: val}
		case "stringSlice":
			val, _ := flagSet.GetStringSlice(f.Name)
			result[f.Name] = v1beta1.ParamValue{Type: v1beta1.ParamTypeArray, ArrayVal: val}
		case "stringToString":
			val, _ := flagSet.GetStringToString(f.Name)
			result[f.Name] = v1beta1.ParamValue{Type: v1beta1.ParamTypeObject, ObjectVal: val}
		}
	})

	return result, nil
}
