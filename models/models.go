package models

type ChromeConfig struct {
	ExePath    string `toml:"exe_path"`
	ProfileDir string `toml:"profile_dir"`
}

type Config struct {
	Output    OutputConfig   `toml:"output"`
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
	Enabled bool `toml:"enabled"`
	AutoExecute bool `toml:"execute"`
	CreateISO bool `toml:"create_iso"`
	MountISO bool `toml:"mount_iso"`
}

