// Copyright 2022 Outreach Corporation. All Rights Reserved.

// Description: This file handles getting SAML identity and parsing it via
// the GitHub API.

package gh

import (
	"context"

	"github.com/getoutreach/goql"
	"github.com/google/go-github/v43/github"
	"github.com/pkg/errors"
)

// OrganizationSAMLIdentity is the struct type that represents the GraphQL query
// to get a SAML identity from a specific organization for a specific user.
type OrganizationSAMLIdentity struct {
	Organization struct {
		SamlIdentityProvider struct {
			ExternalIdentities struct {
				Nodes []struct {
					SamlIdentity struct {
						NameId string `goql:"keep"` //nolint:revive // Why: NameId is how GitHub represents the field, not NameID.
					} `goql:"keep"`
				} `goql:"keep"`
			} `goql:"externalIdentities(first:$first<Int>,login:$user<String!>)"`
		} `goql:"keep"`
	} `goql:"organization(login:$org<String!>)"`
}

// RetrieveSAMLIdentity takes a GitHub client that has been created using the NewClientFromApp
// function. It uses the HTTP transport on the underlying HTTP client from it to authenticate
// the request.
func RetrieveSAMLIdentity(ctx context.Context, client *github.Client, orgLogin, userLogin string) (string, error) {
	gql := goql.NewClient("https://api.github.com/graphql", goql.ClientOptions{
		HTTPClient: client.Client(),
	})

	vars := map[string]interface{}{
		"first": 1,
		"org":   orgLogin,
		"user":  userLogin,
	}

	var orgSamlIdentity OrganizationSAMLIdentity
	if err := gql.Query(ctx, &goql.Operation{
		OperationType: &orgSamlIdentity,
		Variables:     vars,
	}); err != nil {
		return "", errors.Wrap(err, "retrieve saml identity from github graphql api")
	}

	if len(orgSamlIdentity.Organization.SamlIdentityProvider.ExternalIdentities.Nodes) != 1 {
		return "", errors.New("did not receive a proper response back from github graphql api")
	}

	return orgSamlIdentity.Organization.SamlIdentityProvider.ExternalIdentities.Nodes[0].SamlIdentity.NameId, nil
}
