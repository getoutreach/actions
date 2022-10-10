// Copyright 2022 Outreach Corporation. All Rights Reserved.

// Description: See package comment for this one-file package.

// Package opslevel is a package meant to help utilize the OpsLevel API in GitHub actions.
package opslevel

import (
	"fmt"
	"os"
	"strings"

	opslevel "github.com/opslevel/opslevel-go/v2022"
	"github.com/pkg/errors"
)

// Initializes constants for OpsLevel level and lifecycle indexes.
const (
	// BeginnerLevel is the index for the Beginner level in OpsLevel.
	BeginnerLevel = 0
	// BronzeLevel is the index for Bronze level in  OpsLevel.
	BronzeLevel = 1
	// SilverLevel is the index for Silver level in  OpsLevel.
	SilverLevel = 2
	// SilverUpcomingLevel is the index for the Silver (Upcoming) level in OpsLevel.
	SilverUpcomingLevel = 3
	// GoldLevel is the index for Gold level in  OpsLevel.
	GoldLevel = 4
	// PlatinumLevel is the index for Platinum level in  OpsLevel.
	PlatinumLevel = 5

	// DevelopmentLifecycle if the index for the Development lifecycle in OpsLevel.
	DevelopmentLifecycle = 1
	// PrivateBetaLifecycle if the index for the Private Beta lifecycle in OpsLevel.
	PrivateBetaLifecycle = 2
	// PublicBetaLifecycle if the index for the Public Beta lifecycle in OpsLevel.
	PublicBetaLifecycle = 3
	// PublicLifecycle if the index for the Public lifecycle in OpsLevel.
	PublicLifecycle = 4
	// OpsLifecycle if the index for the Ops lifecycle in OpsLevel.
	OpsLifecycle = 5
	// EndOfLifeLifecycle if the index for the Endo-of-Life lifecycle in OpsLevel.
	EndOfLifeLifecycle = 6
)

// NewClient returns a new opslevel client configured with the token we expect to exist in
// the environment.
func NewClient() (*opslevel.Client, error) {
	opslevelToken := strings.TrimSpace(os.Getenv("OPSLEVEL_TOKEN"))
	if opslevelToken == "" {
		return nil, errors.New("OPSLEVEL_TOKEN environment variable is empty")
	}
	return opslevel.NewGQLClient(opslevel.SetAPIToken(opslevelToken)), nil
}

// LifecycleToLevel maps lifecycle index to level index.
// We want to keep this at the index level in case names or other attributes change.
var LifecycleToLevel = map[int]int{
	DevelopmentLifecycle: BeginnerLevel,
	PrivateBetaLifecycle: SilverUpcomingLevel,
	PublicBetaLifecycle:  SilverUpcomingLevel,
	PublicLifecycle:      SilverUpcomingLevel,
	OpsLifecycle:         SilverUpcomingLevel,
	EndOfLifeLifecycle:   BeginnerLevel,
}

// GetServiceAlias safely retrieves the first alias for the provided service.
func GetServiceAlias(service *opslevel.Service) (string, error) {
	if len(service.Aliases) == 0 {
		return "", fmt.Errorf("no aliases present")
	}

	return service.Aliases[0], nil
}

// IsCompliant checks if the service falls within the expected maturity level.
// This check is primarily controlled by the LifecycleToLevel map
func IsCompliant(service *opslevel.Service, sm *opslevel.ServiceMaturity) (bool, error) {
	currentLevelIndex := sm.MaturityReport.OverallLevel.Index
	if len(LifecycleToLevel) <= service.Lifecycle.Index {
		return false, fmt.Errorf("unsupported lifecycle %d %s",
			service.Lifecycle.Index, service.Lifecycle.Name)
	}

	expectedLevelIndex := LifecycleToLevel[service.Lifecycle.Index]
	return currentLevelIndex >= expectedLevelIndex, nil
}

// GetExpectedLevel retrieves the expected maturity level of the service
func GetExpectedLevel(service *opslevel.Service, levels []opslevel.Level) (string, error) {
	if len(LifecycleToLevel) < service.Lifecycle.Index {
		return "", fmt.Errorf("unsupported lifecycle %d %s",
			service.Lifecycle.Index, service.Lifecycle.Name)
	}
	expectedLevelIndex := LifecycleToLevel[service.Lifecycle.Index]

	for _, l := range levels {
		if l.Index == expectedLevelIndex {
			return l.Name, nil
		}
	}

	return "", fmt.Errorf("unable to find level index %d", expectedLevelIndex)
}

// GetLevel retrieves the current maturity report level of the service.
func GetLevel(sm *opslevel.ServiceMaturity) string {
	return sm.MaturityReport.OverallLevel.Name
}

// GetSlackChannel retrieves the slack channel to use for contacting the team responsible for the service.
func GetSlackChannel(team *opslevel.Team) (string, error) {
	for _, c := range team.Contacts {
		if c.Type == opslevel.ContactTypeSlack {
			// We need to drop the leading `#`.
			// This is safe to do with index because it is known to always equal `#`.
			return c.Address[1:], nil
		}
	}

	return "", fmt.Errorf("no slack channel found for team")
}

// GetMaturityReportURL retrieves the html url for the maturity report
func GetMaturityReportURL(service *opslevel.Service) string {
	return service.HtmlURL + "/maturity-report"
}
