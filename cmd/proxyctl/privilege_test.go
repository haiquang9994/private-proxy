package main

import "testing"

func TestCheckRoot(t *testing.T) {
	tests := []struct {
		name    string
		goos    string
		euid    int
		wantErr bool
	}{
		{"root on linux", "linux", 0, false},
		{"normal user on linux", "linux", 1000, true},
		{"windows has no euid", "windows", -1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkRoot(tt.goos, tt.euid)
			if (err != nil) != tt.wantErr {
				t.Fatalf("checkRoot(%q, %d) = %v, wantErr %v", tt.goos, tt.euid, err, tt.wantErr)
			}
			if err != nil && err.Error() != "proxyctl must be run as root (try sudo)" {
				t.Fatalf("unexpected message: %v", err)
			}
		})
	}
}
