package changeguard

import "testing"

func TestBuildScenarioPolicy_PriceResourceTemplate(t *testing.T) {
	scenario := ScenarioConfig{
		Key:  "membership_plan_guard",
		Kind: "price_resource",
		Name: "会员套餐",
		Notify: ScenarioNotifyConfig{
			RecipientProfile: "default_admins",
		},
		Overrides: ScenarioOverrideConfig{
			DisplayNames: map[string]string{
				"custom_price_fen": "自定义价",
			},
		},
	}

	policy, ok := buildScenarioPolicy(scenario.Key, scenario.Kind, scenario)
	if !ok {
		t.Fatalf("expected buildScenarioPolicy success")
	}
	if policy.RiskLevel != RiskLevelHigh {
		t.Fatalf("expected risk level %q, got %q", RiskLevelHigh, policy.RiskLevel)
	}
	if !policy.VersioningEnabled {
		t.Fatalf("expected versioning enabled")
	}
	if !policy.DriftCheckEnabled {
		t.Fatalf("expected drift check enabled")
	}
	if got := policy.DisplayNames["sale_price_fen"]; got != "销售价" {
		t.Fatalf("expected default display name, got %q", got)
	}
	if got := policy.DisplayNames["custom_price_fen"]; got != "自定义价" {
		t.Fatalf("expected merged custom display name, got %q", got)
	}
}

func TestBuildScenarioResource_KindMismatch(t *testing.T) {
	resource, ok, reason := buildScenarioResource("payment_config_guard", "critical_config", "支付配置", ScenarioSourceConfig{
		Type:  "gorm_entity",
		Model: "payment_config",
	}, AppSpec{})
	if ok {
		t.Fatalf("expected buildScenarioResource fail, got resource=%+v", resource)
	}
	if reason == "" {
		t.Fatalf("expected mismatch reason")
	}
}

func TestBuildScenarioBindings_RecipientProfileValidation(t *testing.T) {
	bindings, ok, reason := buildScenarioBindings("payment_config_guard", "critical_config", ScenarioConfig{
		Key:  "payment_config_guard",
		Kind: "critical_config",
		Name: "支付配置",
		Routes: []ScenarioRouteConfig{
			{
				Path:    "/api/v1/admin/config/payment/save",
				Action:  "save",
				Methods: []string{"post"},
			},
		},
		Notify: ScenarioNotifyConfig{
			RecipientProfile: "not_exists",
		},
	}, ProvidersConfig{})
	if ok {
		t.Fatalf("expected buildScenarioBindings fail, got bindings=%+v", bindings)
	}
	if reason == "" {
		t.Fatalf("expected missing recipient profile reason")
	}
}

func TestBuildScenarioBindings_NormalizeMethodsAndMetadata(t *testing.T) {
	providers := ProvidersConfig{
		RecipientProfiles: map[string]RecipientProfileConfig{
			"default_admins": {
				Mode: "fixed_phones",
				Values: []string{
					"18800000000",
				},
			},
		},
	}
	bindings, ok, reason := buildScenarioBindings("payment_config_guard", "critical_config", ScenarioConfig{
		Key:  "payment_config_guard",
		Kind: "critical_config",
		Name: "支付配置",
		Routes: []ScenarioRouteConfig{
			{
				Path:    "/api/v1/admin/config/payment/save",
				Action:  "save",
				Methods: []string{"post"},
			},
		},
		Notify: ScenarioNotifyConfig{
			RecipientProfile: "default_admins",
		},
	}, providers)
	if !ok {
		t.Fatalf("expected buildScenarioBindings success, reason=%s", reason)
	}
	if len(bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(bindings))
	}
	if len(bindings[0].Methods) != 1 || bindings[0].Methods[0] != "POST" {
		t.Fatalf("expected normalized POST methods, got %+v", bindings[0].Methods)
	}
	if got := bindings[0].Metadata["recipient_profile"]; got != "default_admins" {
		t.Fatalf("expected recipient_profile metadata, got %q", got)
	}
}

func TestBuildRecipientResolver_FixedPhones(t *testing.T) {
	resolver, ok := buildRecipientResolver("finance_phones", RecipientProfileConfig{
		Mode:   "fixed_phones",
		Values: []string{"18800000000", "  ", "18800000001"},
	}, AppSpec{})
	if !ok {
		t.Fatalf("expected fixed_phones resolver success")
	}
	values, err := resolver.Resolve(nil, ChangeEvent{}, "sms")
	if err != nil {
		t.Fatalf("expected resolve success, got err=%v", err)
	}
	if len(values) != 2 {
		t.Fatalf("expected 2 recipients, got %d", len(values))
	}
}

func TestBuildScenarioPolicy_SecondFactorOverrides(t *testing.T) {
	enabled := true
	scenario := ScenarioConfig{
		Key:  "payment_config_guard",
		Kind: "critical_config",
		Name: "支付配置",
		Overrides: ScenarioOverrideConfig{
			SecondFactorEnabled:   &enabled,
			SecondFactorMode:      SecondFactorModeSMSCodeOrReply,
			SecondFactorOnActions: []string{ActionSave, ActionPublish},
		},
	}
	policy, ok := buildScenarioPolicy(scenario.Key, scenario.Kind, scenario)
	if !ok {
		t.Fatalf("expected buildScenarioPolicy success")
	}
	if !policy.RequireSecondFactor {
		t.Fatalf("expected require second factor")
	}
	if policy.SecondFactorMode != SecondFactorModeSMSCodeOrReply {
		t.Fatalf("expected second factor mode %q, got %q", SecondFactorModeSMSCodeOrReply, policy.SecondFactorMode)
	}
	if len(policy.SecondFactorOnActions) != 2 {
		t.Fatalf("expected 2 second factor actions, got %d", len(policy.SecondFactorOnActions))
	}
}
