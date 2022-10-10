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
// We want to keep this at the index level in case names or other attributes change
var LifecycleToLevel = map[int]int{
	// In Development >= Beginner
	1: 0,
	// Private Beta >= Silver (Upcoming)
	2: 3,
	// Public Beta >= Silver (Upcoming)
	3: 3,
	// Public/GA >= Silver (Upcoming)
	4: 3,
	// Ops >= Silver (Upcoming)
	5: 3,
	// End-of-life >= Beginner
	6: 0,
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
	if len(LifecycleToLevel) <= service.Lifecycle.Index {
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

	return "", fmt.Errorf("No slach channel found for team")
}

// GetMaturityReportURL retrieves the html url for the maturity report
func GetMaturityReportURL(service *opslevel.Service) string {
	return service.HtmlURL + "/maturity-report"
}
