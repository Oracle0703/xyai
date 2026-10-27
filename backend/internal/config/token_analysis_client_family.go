package config

import (
	"fmt"
	"regexp"
)

// maxTokenAnalysisClientFamilyRules 限制规则数量, 每条请求都会顺序匹配。
const maxTokenAnalysisClientFamilyRules = 64

var tokenAnalysisClientFamilyNamePattern = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)

func validateTokenAnalysisClientFamilyRules(rules []TokenAnalysisClientFamilyRule) error {
	if len(rules) > maxTokenAnalysisClientFamilyRules {
		return fmt.Errorf("token_analysis.client_family_rules must not exceed %d rules", maxTokenAnalysisClientFamilyRules)
	}
	for i, rule := range rules {
		if !tokenAnalysisClientFamilyNamePattern.MatchString(rule.Family) {
			return fmt.Errorf("token_analysis.client_family_rules[%d].family must match ^[a-z0-9_]{1,32}$", i)
		}
		if rule.Pattern == "" {
			return fmt.Errorf("token_analysis.client_family_rules[%d].pattern must not be empty", i)
		}
		if _, err := regexp.Compile("(?i)" + rule.Pattern); err != nil {
			return fmt.Errorf("token_analysis.client_family_rules[%d].pattern is invalid: %w", i, err)
		}
	}
	return nil
}
