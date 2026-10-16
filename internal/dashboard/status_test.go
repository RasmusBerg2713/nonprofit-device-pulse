package dashboard

import (
	"testing"
	"time"
)

func TestDecideVolunteerReminder(t *testing.T) {
	tests := []struct {
		name       string
		online     bool
		battery    int
		wantReason string
		wantReady  int
	}{
		{name: "ready device", online: true, battery: 82, wantReady: 1},
		{name: "low battery needs a visit", online: true, battery: 20, wantReason: "low_battery"},
		{name: "offline takes priority", online: false, battery: 9, wantReason: "device_offline"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := DeviceStatus{
				DeviceID: "tablet-17", CampaignID: "school-drive", DonorID: "donor-42",
				VolunteerID: "volunteer-8", Online: tt.online, BatteryPercent: tt.battery,
				ObservedAt: time.Date(2026, 9, 28, 9, 30, 0, 0, time.UTC),
			}
			got := Decide(status, map[string]any{"value": 12})

			if got.Campaign.DevicesReady != tt.wantReady {
				t.Fatalf("DevicesReady = %d, want %d", got.Campaign.DevicesReady, tt.wantReady)
			}
			if tt.wantReason == "" && got.Reminder != nil {
				t.Fatalf("Reminder = %#v, want nil", got.Reminder)
			}
			if tt.wantReason != "" && (got.Reminder == nil || got.Reminder.Reason != tt.wantReason) {
				t.Fatalf("Reminder = %#v, want reason %q", got.Reminder, tt.wantReason)
			}
			if got.Receipt.Status != "recorded" || got.Receipt.DonorID != "donor-42" {
				t.Fatalf("Receipt = %#v, want recorded donor receipt", got.Receipt)
			}
		})
	}
}
