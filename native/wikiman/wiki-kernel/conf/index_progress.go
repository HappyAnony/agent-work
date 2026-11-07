package conf

import "fmt"

// Original five SQL progress callback bodies, independent of global Conf,
// mobile container, boot details and process-wide message routing.
type SQLProgress struct {
	Kind                       string
	Current, Total, BlockCount int
	Detail                     string
}
type SQLProgressPolicy struct{ InsertBlocks, DeleteBlocks, Mobile bool }

func SQLProgressMessage(p SQLProgress, policy SQLProgressPolicy, language func(int) string) (string, bool) {
	if language == nil {
		return "", false
	}
	switch p.Kind {
	case "blocks":
		if !policy.InsertBlocks {
			return "", false
		}
		return fmt.Sprintf(language(90), p.Current, p.Total, p.BlockCount, p.Detail), true
	case "delete":
		if !policy.DeleteBlocks {
			return "", false
		}
		return fmt.Sprintf(language(93), p.Current, p.Total, p.Detail), true
	case "rename":
		if policy.Mobile {
			return "", false
		}
		return fmt.Sprintf(language(234), p.Current, p.Total, p.BlockCount, p.Detail), true
	case "history":
		if policy.Mobile {
			return "", false
		}
		return fmt.Sprintf(language(191), p.Current, p.Total), true
	case "asset":
		if policy.Mobile {
			return "", false
		}
		return fmt.Sprintf(language(217), p.Current, p.Total), true
	}
	return "", false
}
