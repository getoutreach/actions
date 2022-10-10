package opslevel_test

import (
	"testing"

	"github.com/getoutreach/actions/pkg/opslevel"
	opslevelGo "github.com/opslevel/opslevel-go/v2022"
)

func TestGetServiceAlias(t *testing.T) {
	testCases := []struct {
		name      string
		service   opslevelGo.Service
		expected  string
		expectErr bool
	}{
		{
			name: "single alias",
			service: opslevelGo.Service{
				ServiceId: opslevelGo.ServiceId{
					Aliases: []string{
						"test",
					},
				},
			},
			expected:  "test",
			expectErr: false,
		},
		{
			name: "multiple aliases",
			service: opslevelGo.Service{
				ServiceId: opslevelGo.ServiceId{
					Aliases: []string{
						"test",
						"test2",
					},
				},
			},
			expected:  "test",
			expectErr: false,
		},
		{
			name: "no aliases",
			service: opslevelGo.Service{
				ServiceId: opslevelGo.ServiceId{
					Aliases: []string{},
				},
			},
			expected:  "",
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := opslevel.GetServiceAlias(&tc.service)
			if err != nil {
				if tc.expectErr {
					return
				}
				t.Fatalf("unexpected error")
			}
			if tc.expectErr {
				t.Fatalf("expected and error but did not receive one")
			}

			if result != tc.expected {
				t.Fatalf("expected: %s, got: %s", tc.expected, result)
			}
		})
	}
}

func TestIsComplient(t *testing.T) {
	testCases := []struct {
		name      string
		service   opslevelGo.Service
		sm        *opslevelGo.ServiceMaturity
		expected  bool
		expectErr bool
	}{
		{
			name: "level matches expected level",
			service: opslevelGo.Service{
				Lifecycle: opslevelGo.Lifecycle{
					Index: 4,
				},
			},
			sm: &opslevelGo.ServiceMaturity{
				MaturityReport: opslevelGo.MaturityReport{
					OverallLevel: opslevelGo.Level{
						Index: 3,
					},
				},
			},
			expected:  true,
			expectErr: false,
		},
		{
			name: "level below expected level",
			service: opslevelGo.Service{
				Lifecycle: opslevelGo.Lifecycle{
					Index: 4,
				},
			},
			sm: &opslevelGo.ServiceMaturity{
				MaturityReport: opslevelGo.MaturityReport{
					OverallLevel: opslevelGo.Level{
						Index: 1,
					},
				},
			},
			expected:  false,
			expectErr: false,
		},
		{
			name: "level above expected level",
			service: opslevelGo.Service{
				Lifecycle: opslevelGo.Lifecycle{
					Index: 4,
				},
			},
			sm: &opslevelGo.ServiceMaturity{
				MaturityReport: opslevelGo.MaturityReport{
					OverallLevel: opslevelGo.Level{
						Index: 4,
					},
				},
			},
			expected:  true,
			expectErr: false,
		},
		{
			name: "lifecycle outside supported range",
			service: opslevelGo.Service{
				Lifecycle: opslevelGo.Lifecycle{
					Index: 10,
				},
			},
			sm: &opslevelGo.ServiceMaturity{
				MaturityReport: opslevelGo.MaturityReport{
					OverallLevel: opslevelGo.Level{
						Index: 3,
					},
				},
			},
			expected:  false,
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := opslevel.IsCompliant(&tc.service, tc.sm)
			if err != nil {
				if tc.expectErr {
					return
				}
				t.Fatalf("unexpected error")
			}
			if tc.expectErr {
				t.Fatalf("expected and error but did not receive one")
			}

			if result != tc.expected {
				t.Fatalf("expected: %t, got: %t", tc.expected, result)
			}
		})
	}
}

func TestGetExpectedLevel(t *testing.T) {
	levels := []opslevelGo.Level{
		{
			Index: 0,
			Name:  "Beginner",
		},
		{
			Index: 3,
			Name:  "Silver (Upcoming)",
		},
		{
			Index: 1,
			Name:  "Bronze",
		},
	}
	testCases := []struct {
		name      string
		service   opslevelGo.Service
		expected  string
		expectErr bool
	}{
		{
			name: "level matching index",
			service: opslevelGo.Service{
				Lifecycle: opslevelGo.Lifecycle{
					Index: 0,
				},
			},
			expected:  "Beginner",
			expectErr: false,
		},
		{
			name: "level not matching index",
			service: opslevelGo.Service{
				Lifecycle: opslevelGo.Lifecycle{
					Index: 2,
				},
			},
			expected:  "Silver (Upcoming)",
			expectErr: false,
		},
		{
			name: "unsupported lifecycle",
			service: opslevelGo.Service{
				Lifecycle: opslevelGo.Lifecycle{
					Index: 10,
				},
			},
			expected:  "",
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := opslevel.GetExpectedLevel(&tc.service, levels)
			if err != nil {
				if tc.expectErr {
					return
				}
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.expectErr {
				t.Fatalf("expected and error but did not receive one")
			}

			if result != tc.expected {
				t.Fatalf("expected: %s, got: %s", tc.expected, result)
			}
		})
	}
}

func TestGetLevel(t *testing.T) {
	expected := "Silver"

	sm := &opslevelGo.ServiceMaturity{
		MaturityReport: opslevelGo.MaturityReport{
			OverallLevel: opslevelGo.Level{
				Name: expected,
			},
		},
	}

	result := opslevel.GetLevel(sm)
	if result != expected {
		t.Fatalf("expected: %s, got: %s", expected, result)
	}
}

func TestGetSlackChannel(t *testing.T) {
	testCases := []struct {
		name      string
		team      opslevelGo.Team
		expected  string
		expectErr bool
	}{
		{
			name: "single slack channel",
			team: opslevelGo.Team{
				Contacts: []opslevelGo.Contact{
					{
						Type:    opslevelGo.ContactTypeSlack,
						Address: "#slack-channel",
					},
				},
			},
			expected:  "slack-channel",
			expectErr: false,
		},
		{
			name: "single slack channel with email",
			team: opslevelGo.Team{
				Contacts: []opslevelGo.Contact{
					{
						Type:    opslevelGo.ContactTypeEmail,
						Address: "test@test.com",
					},
					{
						Type:    opslevelGo.ContactTypeSlack,
						Address: "#slack-channel",
					},
				},
			},
			expected:  "slack-channel",
			expectErr: false,
		},
		{
			name: "multiple slack channels",
			team: opslevelGo.Team{
				Contacts: []opslevelGo.Contact{
					{
						Type:    opslevelGo.ContactTypeSlack,
						Address: "#slack-channel",
					},
					{
						Type:    opslevelGo.ContactTypeSlack,
						Address: "#bad-slack-channel",
					},
				},
			},
			expected:  "slack-channel",
			expectErr: false,
		},
		{
			name: "no slack channel",
			team: opslevelGo.Team{
				Contacts: []opslevelGo.Contact{
					{
						Type:        opslevelGo.ContactTypeEmail,
						DisplayName: "test@test.com",
					},
				},
			},
			expected:  "",
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := opslevel.GetSlackChannel(&tc.team)
			if err != nil {
				if tc.expectErr {
					return
				}
				t.Fatalf("unexpected error")
			}
			if tc.expectErr {
				t.Fatalf("expected and error but did not receive one")
			}

			if result != tc.expected {
				t.Fatalf("expected: %s, got: %s", tc.expected, result)
			}
		})
	}
}

func TestGetMaturityReportURL(t *testing.T) {
	expected := "https://app.opslevel.com/services/devtooltestservice/maturity-report"

	service := opslevelGo.Service{
		HtmlURL: "https://app.opslevel.com/services/devtooltestservice",
	}

	result := opslevel.GetMaturityReportURL(&service)
	if result != expected {
		t.Fatalf("expected: %s, got: %s", expected, result)
	}
}
