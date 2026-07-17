package models

type ChromeConfig struct {
	ExePath    string `toml:"exe_path"`
	ProfileDir string `toml:"profile_dir"`
}

type LogConfig struct {
	Handler string `toml:"handler"`
	Info    string `toml:"level"`
	Enabled bool   `toml:"enabled"`
	Where   string `toml:"where"`
}

type Config struct {
	Output    OutputConfig   `toml:"output"`
	Log       LogConfig      `toml:"logging"`
	Chrome    ChromeConfig   `toml:"chrome"`
	Cookies   SectionConfig  `toml:"cookies"`
	Cache     SectionConfig  `toml:"cache"`
	History   SectionConfig  `toml:"history"`
	TempFiles SectionConfig  `toml:"temp_files"`
	Registry  SectionConfig  `toml:"registry"`
	Trash     SectionConfig  `toml:"trash"`
	Shredder  ShredderConfig `toml:"shredder"`
	Firewall  FirewallConfig `toml:"firewall"`
	Virus     VirusConfig    `toml:"virus"`
	CpuLoad   CPULoadConfig  `toml:"cpuload"`
	MemoryLoad MemoryLoadConfig `toml:"memoryload"`
}

type FirewallConfig struct {
	Enabled   bool     `toml:"enabled"`
	Sites     []string `toml:"sites"`
	CallTimes int      `toml:"call_times"`
}

type OutputConfig struct {
	BaseDir string `toml:"base_dir"`
}

type SectionConfig struct {
	Enabled bool `toml:"enabled"`
	Count   int  `toml:"count"`
}

type ShredderTempFilesConfig struct {
	Count int `toml:"count"`
}

type ShredderConfig struct {
	Enabled   bool                    `toml:"enabled"`
	TempFiles ShredderTempFilesConfig `toml:"tempfiles"`
}

type VirusConfig struct {
	Enabled     bool `toml:"enabled"`
	AutoExecute bool `toml:"execute"`
	CreateISO   bool `toml:"create_iso"`
	MountISO    bool `toml:"mount_iso"`
}

type CPULoadConfig struct {
	Enabled        bool `toml:"enabled"`
	PercentageLoad int  `toml:"percentage"`
	Cores int `toml:"cores"`
}


type MemoryLoadConfig struct {
	Enabled bool `toml:"enabled"`
	TargetMB int // amount of memory to hold, in MB
}
