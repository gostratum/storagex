package storagex

import (
	"testing"
)

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "explicit creds present",
			cfg: &Config{
				Provider:  "s3",
				Bucket:    "my-bucket",
				Region:    "us-east-1",
				AccessKey: "AKIA...",
				SecretKey: "secret",
			},
			wantErr: false,
		},
		{
			name: "one cred missing",
			cfg: &Config{
				Provider:  "s3",
				Bucket:    "my-bucket",
				Region:    "us-east-1",
				AccessKey: "",
				SecretKey: "secret",
			},
			wantErr: true,
		},
		{
			name: "empty creds non-aws endpoint and no sdk defaults",
			cfg: &Config{
				Provider: "s3",
				Bucket:   "my-bucket",
				Endpoint: "http://minio.local:9000",
			},
			wantErr: true,
		},
		{
			name: "empty creds with use sdk defaults",
			cfg: &Config{
				Provider:       "s3",
				Bucket:         "my-bucket",
				UseSDKDefaults: true,
			},
			wantErr: false,
		},
		{
			name: "role arn present with empty creds",
			cfg: &Config{
				Provider: "s3",
				Bucket:   "my-bucket",
				RoleARN:  "arn:aws:iam::123456789012:role/TestRole",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
