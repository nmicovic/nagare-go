//go:build linux || freebsd || netbsd || openbsd || solaris || dragonfly

package clipboard

import "testing"

func TestDetectionRunsOnFirstUse(t *testing.T) {
	if pasteCmdArgs != nil || copyCmdArgs != nil {
		t.Fatal("clipboard utilities were detected during package initialization")
	}
	if Unsupported {
		t.Fatal("clipboard support was decided during package initialization")
	}

	t.Setenv("PATH", t.TempDir())
	if _, err := ReadAll(); err == nil {
		t.Fatal("ReadAll succeeded without a clipboard utility")
	}
	if !Unsupported {
		t.Fatal("first clipboard use did not record unsupported environment")
	}
	if pasteCmdArgs == nil || copyCmdArgs == nil {
		t.Fatal("first clipboard use did not run detection")
	}
}
