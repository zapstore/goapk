package config

import "testing"

func TestValidate(t *testing.T) {
	good := &BuildConfig{
		AppName:     "My App",
		PackageName: "com.example.myapp",
		VersionCode: 1,
		VersionName: "1.0",
		MinSDK:      24,
		TargetSDK:   35,
		AssetsDir:   "/tmp/assets",
		IconColor:   "icon.png",
		OutputPath:  "app.apk",
	}
	if err := good.Validate(); err != nil {
		t.Errorf("Validate(good) = %v, want nil", err)
	}

	tests := []struct {
		name    string
		mutate  func(*BuildConfig)
		wantErr string
	}{
		{"empty name", func(c *BuildConfig) { c.AppName = "" }, "app name"},
		{"bad package", func(c *BuildConfig) { c.PackageName = "invalid" }, "package name"},
		{"low minsdk", func(c *BuildConfig) { c.MinSDK = 21 }, "min SDK"},
		{"no content", func(c *BuildConfig) { c.AssetsDir = ""; c.RemoteURL = "" }, "source"},
		{"no icon", func(c *BuildConfig) { c.IconColor = "" }, "icon"},
		{"no output", func(c *BuildConfig) { c.OutputPath = "" }, "output"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := *good
			tt.mutate(&c)
			err := c.Validate()
			if err == nil {
				t.Errorf("Validate() = nil, want error containing %q", tt.wantErr)
			}
		})
	}
}

func TestDefaults(t *testing.T) {
	c := &BuildConfig{}
	c.Defaults()
	if c.VersionCode != 1 {
		t.Errorf("VersionCode = %d, want 1", c.VersionCode)
	}
	if c.VersionName != "1.0" {
		t.Errorf("VersionName = %q, want 1.0", c.VersionName)
	}
	if c.MinSDK != 24 {
		t.Errorf("MinSDK = %d, want 24", c.MinSDK)
	}
	if c.TargetSDK != 35 {
		t.Errorf("TargetSDK = %d, want 35", c.TargetSDK)
	}
}
