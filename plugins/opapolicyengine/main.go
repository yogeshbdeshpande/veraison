// Copyright 2021 Contributors to the Veraison project.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/go-plugin"
	"github.com/open-policy-agent/opa/rego"

	"github.com/veraison/common"
)

type OpaPolicyEngine struct {
	ctx    context.Context
	policy func(r *rego.Rego)
}

func (pe *OpaPolicyEngine) GetName() string {
	return "opa"
}

// Init initializes the OPA context that will be used to evaluate the policy.
// It does not expect any arguments.
func (pe *OpaPolicyEngine) Init(args common.PolicyEngineParams) error {
	ctx := context.Background()
	pe.ctx = ctx
	pe.policy = nil
	return nil
}

func (pe *OpaPolicyEngine) LoadPolicy(policy []byte) error {
	pe.policy = rego.Module("policy", string(policy))
	return nil // TODO: can validate at this point somehow?
}

func (pe *OpaPolicyEngine) CheckValid(
	evidence map[string]interface{},
	endorsements map[string]interface{},
) (common.Status, error) {
	if pe.policy == nil {
		return common.StatusFailure, fmt.Errorf("policy not set")
	}

	input := map[string]interface{}{"evidence": evidence, "endorsements": endorsements}

	rego := rego.New(
		rego.Query("data.iat.allow"),
		rego.Input(input),
		pe.policy)

	rs, err := rego.Eval(pe.ctx)
	if err != nil {
		return common.StatusFailure, err
	}

	result := rs[0].Expressions[0].Value
	switch t := result.(type) {
	case bool:
		if result.(bool) {
			return common.StatusSuccess, nil
		}

		return common.StatusFailure, nil
	default:
		return common.StatusFailure, fmt.Errorf("query evaluated to %v; expected bool", t)
	}
}

func (pe *OpaPolicyEngine) GetClaims(
	evidence map[string]interface{},
	endorsements map[string]interface{},
) (map[string]interface{}, error) {
	if pe.policy == nil {
		return nil, fmt.Errorf("policy not set")
	}

	input := map[string]interface{}{"evidence": evidence, "endorsements": endorsements}

	rego := rego.New(
		rego.Query("data.iat.evidence"),
		rego.Input(input),
		pe.policy)

	rs, err := rego.Eval(pe.ctx)
	if err != nil {
		return nil, err
	}

	result := rs[0].Expressions[0].Value
	switch t := result.(interface{}).(type) {
	case map[string]interface{}:
		return result.(map[string]interface{}), nil
	default:
		return nil, fmt.Errorf("query evaluated to %v; expected map[string]interface{}", t)
	}
}

func (pe *OpaPolicyEngine) GetAttetationResult(
	evidence map[string]interface{},
	endorsements map[string]interface{},
	simple bool,
	result *common.AttestationResult,
) error {
	var err error

	result.Status, err = pe.CheckValid(evidence, endorsements)
	if err != nil {
		return err
	}

	if simple {
		return nil
	}

	claimsLabel := common.NewStringLabel("veraison-processed-evidence")
	result.ProcessedEvidence[claimsLabel], err = pe.GetClaims(evidence, endorsements)
	if err != nil {
		return err
	}

	return nil
}

// Stop is a no-op for this plugin.
func (pe *OpaPolicyEngine) Stop() error {
	return nil
}

func main() {
	var handshakeConfig = plugin.HandshakeConfig{
		ProtocolVersion:  1,
		MagicCookieKey:   "VERAISON_PLUGIN",
		MagicCookieValue: "VERAISON",
	}

	var pluginMap = map[string]plugin.Plugin{
		"policyengine": &common.PolicyEnginePlugin{
			Impl: &OpaPolicyEngine{},
		},
	}

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: handshakeConfig,
		Plugins:         pluginMap,
	})
}
