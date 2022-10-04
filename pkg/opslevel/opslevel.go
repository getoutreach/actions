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
	"github.com/shurcooL/graphql"
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
	// Private Beta >= Silver
	2: 2,
	// Public Beta >= Silver
	3: 2,
	// Public/GA >= Silver
	4: 2,
	// Ops >= Silver
	5: 2,
	// End-of-life >= Beginner
	6: 0,
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
			return c.Address, nil
		}
	}

	return "", fmt.Errorf("No slach channel found for team")
}

// GetMaturityReportURL retrieves the html url for the maturity report
func GetMaturityReportURL(service *opslevel.Service) string {
	return service.HtmlURL + "/maturity-report"
}

// GetRepositoryID retrieves the first repository id for the given service.
func GetRepositoryID(service *opslevel.Service) (graphql.ID, error) {
	repos := service.Repositories.Edges

	if len(repos) == 0 {
		return "", fmt.Errorf("no repositories linked to service")
	}

	return repos[0].Node.Id, nil
}
