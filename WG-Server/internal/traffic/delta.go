// Package traffic mengubah penghitung kumulatif kernel menjadi pemakaian per jam yang bisa dijumlah.
package traffic

// Counter adalah pembacaan rx/tx kumulatif satu peer dari kernel.
type Counter struct {
	ReceiveBytes  int64
	TransmitBytes int64
}

// Delta menghitung pertambahan byte antara dua pembacaan berturut-turut.
// Kernel mereset penghitung ke 0 ketika interface dibuat ulang (reboot, restart panel yang membuat
// ulang wg0, atau peer dihapus lalu ditambah lagi). Bila nilai sekarang lebih kecil dari sebelumnya,
// berarti terjadi reset, sehingga seluruh nilai sekarang dianggap trafik baru — bukan selisih negatif.
// Byte yang lewat di antara pembacaan terakhir dan reset memang hilang; batas ini diterima (≤ 1 interval).
func Delta(previous, current Counter) Counter {
	return Counter{
		ReceiveBytes:  deltaOne(previous.ReceiveBytes, current.ReceiveBytes),
		TransmitBytes: deltaOne(previous.TransmitBytes, current.TransmitBytes),
	}
}

// deltaOne menerapkan aturan reset untuk satu arah penghitung.
func deltaOne(previous, current int64) int64 {
	if current < previous {
		return current
	}
	return current - previous
}
