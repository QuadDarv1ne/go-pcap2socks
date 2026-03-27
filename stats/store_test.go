package stats

import (
	"testing"
)

func TestNewStore(t *testing.T) {
	store := NewStore()
	if store == nil {
		t.Fatal("Expected non-nil store")
	}
}

func TestRecordTraffic(t *testing.T) {
	store := NewStore()
	ip := "192.168.137.100"
	mac := "78:c8:81:4e:55:15"
	bytes := uint64(1024)

	store.RecordTraffic(ip, mac, bytes, true) // upload

	device := store.GetDeviceStats(ip)
	if device == nil {
		t.Fatal("Expected device to be created")
	}

	if device.GetUploadBytes() != bytes {
		t.Errorf("Expected UploadBytes %d, got %d", bytes, device.GetUploadBytes())
	}

	if device.GetTotalBytes() != bytes {
		t.Errorf("Expected TotalBytes %d, got %d", bytes, device.GetTotalBytes())
	}

	if device.GetPackets() != 1 {
		t.Errorf("Expected Packets 1, got %d", device.GetPackets())
	}
}

func TestRecordTraffic_Download(t *testing.T) {
	store := NewStore()
	ip := "192.168.137.100"
	mac := "78:c8:81:4e:55:15"
	bytes := uint64(2048)

	store.RecordTraffic(ip, mac, bytes, false) // download

	device := store.GetDeviceStats(ip)
	if device.GetDownloadBytes() != bytes {
		t.Errorf("Expected DownloadBytes %d, got %d", bytes, device.GetDownloadBytes())
	}
}

func TestGetTotalTraffic(t *testing.T) {
	store := NewStore()

	store.RecordTraffic("192.168.137.100", "78:c8:81:4e:55:15", 1000, true)
	store.RecordTraffic("192.168.137.100", "78:c8:81:4e:55:15", 2000, false)
	store.RecordTraffic("192.168.137.101", "aa:bb:cc:dd:ee:ff", 500, true)
	store.RecordTraffic("192.168.137.101", "aa:bb:cc:dd:ee:ff", 1500, false)

	total, upload, download, packets := store.GetTotalTraffic()

	expectedUpload := uint64(1500)
	expectedDownload := uint64(3500)
	expectedTotal := expectedUpload + expectedDownload
	expectedPackets := uint64(4)

	if upload != expectedUpload {
		t.Errorf("Expected upload %d, got %d", expectedUpload, upload)
	}
	if download != expectedDownload {
		t.Errorf("Expected download %d, got %d", expectedDownload, download)
	}
	if total != expectedTotal {
		t.Errorf("Expected total %d, got %d", expectedTotal, total)
	}
	if packets != expectedPackets {
		t.Errorf("Expected packets %d, got %d", expectedPackets, packets)
	}
}

func TestSetDisconnected(t *testing.T) {
	store := NewStore()
	ip := "192.168.137.100"
	mac := "78:c8:81:4e:55:15"

	// Create device
	store.RecordTraffic(ip, mac, 100, true)

	device := store.GetDeviceStats(ip)
	if !device.Connected {
		t.Error("Expected device to be connected")
	}

	// Mark as disconnected
	store.SetDisconnected(ip)

	device = store.GetDeviceStats(ip)
	if device.Connected {
		t.Error("Expected device to be disconnected")
	}
}

func TestGetActiveDeviceCount(t *testing.T) {
	store := NewStore()

	store.RecordTraffic("192.168.137.100", "78:c8:81:4e:55:15", 100, true)
	store.RecordTraffic("192.168.137.101", "aa:bb:cc:dd:ee:ff", 200, true)
	store.RecordTraffic("192.168.137.102", "11:22:33:44:55:66", 300, true)

	// Disconnect one device
	store.SetDisconnected("192.168.137.101")

	activeCount := store.GetActiveDeviceCount()
	if activeCount != 2 {
		t.Errorf("Expected 2 active devices, got %d", activeCount)
	}
}

func TestGetAllDevices(t *testing.T) {
	store := NewStore()

	store.RecordTraffic("192.168.137.100", "78:c8:81:4e:55:15", 100, true)
	store.RecordTraffic("192.168.137.101", "aa:bb:cc:dd:ee:ff", 200, true)

	devices := store.GetAllDevices()
	if len(devices) != 2 {
		t.Errorf("Expected 2 devices, got %d", len(devices))
	}
}

func TestExportCSV(t *testing.T) {
	store := NewStore()

	store.RecordTraffic("192.168.137.100", "78:c8:81:4e:55:15", 1024, true)
	store.RecordTraffic("192.168.137.100", "78:c8:81:4e:55:15", 2048, false)

	csv, err := store.ExportCSV()
	if err != nil {
		t.Fatalf("ExportCSV() error = %v", err)
	}

	if csv == "" {
		t.Error("Expected non-empty CSV")
	}
}

func TestReset(t *testing.T) {
	store := NewStore()

	store.RecordTraffic("192.168.137.100", "78:c8:81:4e:55:15", 100, true)
	store.Reset()

	devices := store.GetAllDevices()
	if len(devices) != 0 {
		t.Errorf("Expected 0 devices after reset, got %d", len(devices))
	}
}

func TestGetConnectedDevices(t *testing.T) {
	store := NewStore()

	store.RecordTraffic("192.168.137.100", "78:c8:81:4e:55:15", 100, true)
	store.RecordTraffic("192.168.137.101", "aa:bb:cc:dd:ee:ff", 200, true)
	store.SetDisconnected("192.168.137.101")

	connected := store.GetConnectedDevices()
	if len(connected) != 1 {
		t.Errorf("Expected 1 connected device, got %d", len(connected))
	}
}
