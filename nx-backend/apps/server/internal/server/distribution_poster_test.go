package server

import "testing"

func TestPosterConfigValidation(t *testing.T) {
	base := posterConfig{TemplateURL: "/api/upload-assets/1", LandingURL: "https://example.com/fixed", QRSize: 176}
	if err := validatePosterConfig(base); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*posterConfig){
		"missing template": func(c *posterConfig) { c.TemplateURL = "" },
		"missing QR":       func(c *posterConfig) { c.LandingURL = "" },
		"script QR":        func(c *posterConfig) { c.LandingURL = "javascript:alert(1)" },
		"untrusted image":  func(c *posterConfig) { c.TemplateURL = "https://example.com/image.png" },
		"traversal":        func(c *posterConfig) { c.TemplateURL = "/api/uploads/../secret" },
		"oversize QR":      func(c *posterConfig) { c.QRSize = 221 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := base
			mutate(&c)
			if validatePosterConfig(c) == nil {
				t.Fatal("accepted invalid configuration")
			}
		})
	}
	t.Run("fixed image without URL", func(t *testing.T) {
		c := base
		c.LandingURL = ""
		c.QRImageURL = "/api/upload-assets/2"
		if err := validatePosterConfig(c); err != nil {
			t.Fatal(err)
		}
	})
}
