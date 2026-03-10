package runner

import (
	"os"
	"strings"
)

type IOStep struct {
	reportOutput string
	envs         []string
	secretEnvs   []string
}

func WithIO(reportOutput string, envs, secretEnvs []string) *IOStep {
	return &IOStep{reportOutput: reportOutput, envs: envs, secretEnvs: secretEnvs}
}

func (s *IOStep) Run(rc *RunContext, next Next) error {
	reportOutput := s.reportOutput
	if reportOutput == "/dev/stdout" || reportOutput == "" {
		rc.ReportDev = rc.Stdout
	} else {
		f, err := os.OpenFile(reportOutput, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0640)
		if err != nil {
			return err
		}
		rc.ReportDev = rc.SecretStore.Writer(f)
	}

	rc.Envs = s.envMap(s.envs)
	rc.Secrets = s.envMap(s.secretEnvs)
	for _, secretValue := range rc.Secrets {
		rc.SecretStore.AddSecrets([]byte(secretValue))
	}

	return next(rc)
}

func (s *IOStep) envMap(from []string) map[string]string {
	envs := make(map[string]string)
	for _, v := range from {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) == 1 {
			if env, ok := os.LookupEnv(parts[0]); ok {
				envs[parts[0]] = env
			}
		} else {
			envs[parts[0]] = parts[1]
		}
	}
	return envs
}
