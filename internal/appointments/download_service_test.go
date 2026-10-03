package appointments

import (
	"context"
	"errors"
	"testing"
)

type fakeFiles struct {
	valid        bool
	userID       string
	verifyErr    error
	presignCalls int
	lastKey      string
}

func (f *fakeFiles) VerifySession(context.Context, string) (bool, string, error) {
	return f.valid, f.userID, f.verifyErr
}

func (f *fakeFiles) PresignDownload(_ context.Context, _, key, _ string, _ int) (string, error) {
	f.presignCalls++
	f.lastKey = key
	return "https://download.example/signed", nil
}

func TestIssueLinkEnforcesPatientOwnership(t *testing.T) {
	tests := []struct {
		name         string
		valid        bool
		sessionUser  string
		wantErr      error
		wantPresigns int
		wantNotice   string
	}{
		{"matching patient receives link", true, "patient-7", nil, 1, "Your appointment was updated. Open the care portal for the latest details."},
		{"valid session for another patient is denied", true, "patient-8", ErrUnauthorized, 0, ""},
		{"inactive session is denied", false, "patient-7", ErrUnauthorized, 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := &fakeFiles{valid: tt.valid, userID: tt.sessionUser}
			service := NewService(files, "care-private-files")
			got, err := service.IssueLink(context.Background(), DownloadRequest{
				RequestID:         "req-1042",
				SessionID:         "session-9",
				PatientID:         "patient-7",
				AppointmentID:     "appointment-42",
				FileKind:          "visit-summary",
				AppointmentStatus: "rescheduled",
			})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if files.presignCalls != tt.wantPresigns {
				t.Fatalf("presign calls = %d, want %d", files.presignCalls, tt.wantPresigns)
			}
			if got.Notification != tt.wantNotice {
				t.Fatalf("notification = %q, want %q", got.Notification, tt.wantNotice)
			}
			if tt.wantPresigns == 1 && files.lastKey != "patients/patient-7/appointments/appointment-42/visit-summary.pdf" {
				t.Fatalf("object key = %q", files.lastKey)
			}
		})
	}
}
