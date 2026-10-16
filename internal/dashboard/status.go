package dashboard

import "time"

type DeviceStatus struct {
	DeviceID       string    `json:"device_id"`
	CampaignID     string    `json:"campaign_id"`
	DonorID        string    `json:"donor_id"`
	VolunteerID    string    `json:"volunteer_id"`
	Online         bool      `json:"online"`
	BatteryPercent int       `json:"battery_percent"`
	ObservedAt     time.Time `json:"observed_at"`
}

type Receipt struct {
	DonorID    string `json:"donor_id"`
	DeviceID   string `json:"device_id"`
	CampaignID string `json:"campaign_id"`
	Status     string `json:"status"`
}

type VolunteerReminder struct {
	VolunteerID string `json:"volunteer_id"`
	DeviceID    string `json:"device_id"`
	Reason      string `json:"reason"`
}

type CampaignReport struct {
	CampaignID   string `json:"campaign_id"`
	DevicesSeen  int    `json:"devices_seen"`
	DevicesReady int    `json:"devices_ready"`
}

type Update struct {
	Device   DeviceStatus       `json:"device"`
	Receipt  Receipt            `json:"receipt"`
	Reminder *VolunteerReminder `json:"volunteer_reminder,omitempty"`
	Campaign CampaignReport     `json:"campaign"`
	Metrics  any                `json:"metrics"`
}

func Decide(status DeviceStatus, metrics any) Update {
	update := Update{
		Device: status,
		Receipt: Receipt{
			DonorID:    status.DonorID,
			DeviceID:   status.DeviceID,
			CampaignID: status.CampaignID,
			Status:     "recorded",
		},
		Campaign: CampaignReport{
			CampaignID:   status.CampaignID,
			DevicesSeen:  1,
			DevicesReady: boolInt(status.Online && status.BatteryPercent > 20),
		},
		Metrics: metrics,
	}

	if !status.Online {
		update.Reminder = &VolunteerReminder{VolunteerID: status.VolunteerID, DeviceID: status.DeviceID, Reason: "device_offline"}
	} else if status.BatteryPercent <= 20 {
		update.Reminder = &VolunteerReminder{VolunteerID: status.VolunteerID, DeviceID: status.DeviceID, Reason: "low_battery"}
	}

	return update
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
