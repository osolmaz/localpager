package localpager

import "strings"

func shouldNotify(output ClassifierOutput, opts WorkerOptions) bool {
	if opts.NotifyConfidenceMin > 0 && output.Confidence < opts.NotifyConfidenceMin {
		return false
	}
	if interestBlocked(output.Interest, opts.NotifyInterestNot) {
		return false
	}
	if len(opts.NotifyTopicsAny) == 0 {
		return true
	}
	return hasAllowedTopic(output.TopicsOfInterest, opts.NotifyTopicsAny)
}

func hasAllowedTopic(topics, allowedTopics []string) bool {
	allowed := map[string]bool{}
	for _, topic := range allowedTopics {
		normalized := normalizeTopic(topic)
		if normalized != "" {
			allowed[normalized] = true
		}
	}
	for _, topic := range topics {
		if allowed[normalizeTopic(topic)] {
			return true
		}
	}
	return false
}

func interestBlocked(interest string, blocked []string) bool {
	normalized := strings.ToLower(strings.TrimSpace(interest))
	if len(blocked) == 0 {
		return defaultBlockedInterest(normalized)
	}
	for _, value := range blocked {
		if normalized == strings.ToLower(strings.TrimSpace(value)) {
			return true
		}
	}
	return false
}

func defaultBlockedInterest(interest string) bool {
	switch interest {
	case "", "none", "no", "low", "irrelevant", "i0", "false":
		return true
	default:
		return false
	}
}

func normalizeTopic(topic string) string {
	return strings.ToLower(strings.TrimSpace(topic))
}
