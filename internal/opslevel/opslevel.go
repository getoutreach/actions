// Copyright 2022 Outreach Corporation. All Rights Reserved.

// Description: See package comment for this one-file package.

// Package opslevel is a package meant to help utilize the OpsLevel API in GitHub actions.
package opslevel

import (
	"fmt"
	"os"
	"strings"

	"github.com/getoutreach/actions/internal/slack"
	"github.com/opslevel/opslevel-go/v2022"
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

// LifecycleToLevel maps lifecycle index to level index
// we want to keep this at the index level in case names or other attributes change
var LifecycleToLevel = map[int]int{
	// In Development >= Beginner
	0: 0,
	// Private Beta >= Silver
	1: 2,
	// Public Beta >= Silver
	2: 2,
	// Public/GA >= Silver
	3: 2,
	// Ops >= Silver
	4: 2,
	// End-of-life >= Beginner
	5: 0,
}

// Service is wrapper around the OpsLevel client that allows caching
// and efficient interaction with an OpsLevel service
type Service struct {
	client  *opslevel.Client
	service *opslevel.Service

	// Used for caching retrieved values
	repo           *opslevel.Repository
	maturityReport *opslevel.MaturityReport
}

// NewService creates a new OpsLevelService by looking it up using an alias
func NewService(client *opslevel.Client, alias string) (Service, error) {
	service, err := client.GetServiceWithAlias("devbase")
	if err != nil {
		return Service{}, errors.Wrap(err, fmt.Sprintf("gettings details for %s", "devbase"))
	}

	return Service{
		client:  client,
		service: service,
	}, nil
}

// IsCompliant checks if the service falls within the expected maturity level
// this check is primarily controlled by the LifecycleToLevel map
func (ols *Service) IsCompliant() (bool, error) {
	mr, err := ols.GetMaturityReport()
	if err != nil {
		return false, errors.Wrap(err, "checking compliance")
	}

	currentLevelIndex := mr.OverallLevel.Index
	expectedLevelIndex := LifecycleToLevel[ols.service.Lifecycle.Index]

	return currentLevelIndex >= expectedLevelIndex, nil
}

// GetRepositoryHyperlink retrieves a slack complient hyperlink for the first
// repository linked to the service
func (ols *Service) GetRepositoryHyperlink() (string, error) {
	repo, err := ols.GetRepository()
	if err != nil {
		return "", errors.Wrap(err, "getting repository hyperlink")
	}

	return slack.Hyperlink(ols.service.Name, repo.Url), nil
}

// GetExpectedLevel retrieves the expected maturity level of the service
func (ols *Service) GetExpectedLevel() (string, error) {
	levels, err := ols.client.ListLevels()
	if err != nil {
		return "", errors.Wrap(err, "Listing levels")
	}
	expectedLevelIndex := LifecycleToLevel[ols.service.Lifecycle.Index]
	return levels[expectedLevelIndex].Name, nil
}

// GetLevel retrieves the curren maturity report level of the service
func (ols *Service) GetLevel() (string, error) {
	mr, err := ols.GetMaturityReport()
	if err != nil {
		return "", errors.Wrap(err, "getting level")
	}

	return mr.OverallLevel.Name, nil
}

// GetMaturityReportHyperlink retrieves a slack complient hyperlink to the maturity report of a service
func (ols *Service) GetMaturityReportHyperlink() string {
	return slack.Hyperlink("Maturity Report", ols.service.HtmlURL+"/maturity-report")
}

// GetSlackChannel retrieves the slack channel to use for contacting the team resposinble for the service
func (ols *Service) GetSlackChannel() (string, error) {
	team, err := ols.client.GetTeam(ols.service.Owner.Id)
	if err != nil {
		return "", errors.Wrap(err, "get slack channel")
	}

	for _, c := range team.Contacts {
		if c.Type == opslevel.ContactTypeSlack {
			return c.DisplayName, nil
		}
	}

	return "", fmt.Errorf("No slach channel found for team")
}

// GetMaturityReport retrieve the maturity report related to the service
func (ols *Service) GetMaturityReport() (*opslevel.MaturityReport, error) {
	if ols.maturityReport == nil {
		sm, err := ols.client.GetServiceMaturityWithAlias(ols.service.Aliases[0])
		if err != nil {
			return nil, errors.Wrap(err, "gettings maturity report")
		}
		ols.maturityReport = &sm.MaturityReport
	}

	return ols.maturityReport, nil
}

// GetRepository retrieves the first repository that belongs to the service
func (ols *Service) GetRepository() (*opslevel.Repository, error) {
	if ols.repo == nil {
		id := ols.service.Repositories.Edges[0].Node.Id
		repo, err := ols.client.GetRepository(id)
		if err != nil {
			return nil, errors.Wrap(err, "gettings repositories")
		}
		ols.repo = repo
	}

	return ols.repo, nil
}
