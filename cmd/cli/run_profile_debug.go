package main

func (f *runFlags) debugProfile() error {
	if !runCmd.Flags().Changed("report") {
		f.report = reportTypeTable.String()
	}

	if !runCmd.Flags().Changed("expand") {
		f.expand = true
	}

	if !runCmd.Flags().Changed("pull") {
		f.pull = pullImageAlways.String()
	}

	if !runCmd.Flags().Changed("skip-done") {
		f.skipDone = false
	}

	if !runCmd.Flags().Changed("no-gc") {
		f.noGC = true
	}

	if !runCmd.Root().PersistentFlags().Changed("verbose") {
		rootArgs.logOptions.Verbose = 10
		var err error
		logger, zapConfig, err = rootArgs.logOptions.Build()
		if err != nil {
			return err
		}
	}

	return nil
}
