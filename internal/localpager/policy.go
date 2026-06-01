package localpager

import "strings"

func shouldNotify(output ClassifierOutput, opts WorkerOptions) bool {
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

func normalizeTopic(topic string) string {
	return strings.ToLower(strings.TrimSpace(topic))
}
