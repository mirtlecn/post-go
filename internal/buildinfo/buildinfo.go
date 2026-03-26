package buildinfo

import "runtime/debug"

var (
	Version   = "v1.4.5"
	Commit    = "unknown"
	BuildDate = "unknown"
)

type Info struct {
	Version   string
	Commit    string
	BuildDate string
}

var readBuildInfo = debug.ReadBuildInfo

func Current() Info {
	info := Info{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
	}

	buildInfo, ok := readBuildInfo()
	if !ok {
		return info
	}

	if info.Version == "dev" && buildInfo.Main.Version != "" && buildInfo.Main.Version != "(devel)" {
		info.Version = buildInfo.Main.Version
	}

	for _, setting := range buildInfo.Settings {
		switch setting.Key {
		case "vcs.revision":
			if info.Commit == "unknown" && setting.Value != "" {
				info.Commit = setting.Value
			}
		case "vcs.time":
			if info.BuildDate == "unknown" && setting.Value != "" {
				info.BuildDate = setting.Value
			}
		}
	}

	return info
}
