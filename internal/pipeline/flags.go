package pipeline

import (
	"github.com/spf13/pflag"

	"github.com/raffis/rageta/pkg/apis/core/v1beta1"
)

func Flags(set *pflag.FlagSet, params []v1beta1.InputParam) {
	for _, input := range params {
		input.SetDefaults()
		switch input.Type {
		case v1beta1.ParamTypeString:
			var defaultValue string
			if input.Default != nil {
				defaultValue = input.Default.StringVal
			}
			set.String(input.Name, defaultValue, input.Description)
		case v1beta1.ParamTypeArray:
			var defaultValue []string
			if input.Default != nil {
				defaultValue = input.Default.ArrayVal
			}
			set.StringSlice(input.Name, defaultValue, input.Description)
		case v1beta1.ParamTypeObject:
			var defaultValue map[string]string
			if input.Default != nil {
				defaultValue = input.Default.ObjectVal
			}
			set.StringToString(input.Name, defaultValue, input.Description)
		}
	}
}
