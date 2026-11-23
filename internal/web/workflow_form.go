package web

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"tagent/internal/workflow"
)

// Nil fields are untouched. Steps is an explicit sequential reorder/add/remove.
type workflowPatch struct {
	Fields map[string]string
	Jobs   map[string]map[string]string
	Steps  *[]string
}

func mappingValue(n *yaml.Node, key string) *yaml.Node {
	for i := 0; i < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}
func setValue(n *yaml.Node, key, value string) {
	v := mappingValue(n, key)
	if v == nil {
		v = &yaml.Node{}
		n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, v)
	}
	v.Kind = yaml.ScalarNode
	v.Tag = "!!str"
	v.Value = value
	v.Content = nil
}
func patchWorkflow(source string, patch *workflowPatch) (string, workflow.Workflow, error) {
	original, err := workflow.Parse([]byte(source))
	if err != nil {
		return source, original, err
	}
	if patch == nil {
		return source, original, nil
	}
	var doc yaml.Node
	if err = yaml.Unmarshal([]byte(source), &doc); err != nil {
		return source, original, err
	}
	root := doc.Content[0]
	jobs := mappingValue(root, "jobs")
	for key, value := range patch.Fields {
		if key != "name" && key != "description" {
			return source, original, fmt.Errorf("unsupported field %s", key)
		}
		setValue(root, key, value)
	}
	if patch.Steps != nil {
		content := []*yaml.Node{}
		seen := map[string]bool{}
		for i, id := range *patch.Steps {
			if id == "" || seen[id] {
				return source, original, fmt.Errorf("step IDs must be unique and nonempty")
			}
			seen[id] = true
			node := mappingValue(jobs, id)
			if node == nil {
				node = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
				setValue(node, "agent", "implementer")
			}
			if i > 0 {
				setValue(node, "needs", (*patch.Steps)[i-1])
			} else {
				for k := 0; k < len(node.Content); k += 2 {
					if node.Content[k].Value == "needs" {
						node.Content = append(node.Content[:k], node.Content[k+2:]...)
						break
					}
				}
			}
			content = append(content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: id}, node)
		}
		jobs.Content = content
	}
	for id, fields := range patch.Jobs {
		node := mappingValue(jobs, id)
		if node == nil {
			return source, original, fmt.Errorf("unknown step %s", id)
		}
		for key, value := range fields {
			switch key {
			case "agent", "model", "prompt", "run", "on-failure", "status":
				setValue(node, key, value)
			default:
				return source, original, fmt.Errorf("unsupported step field %s", key)
			}
		}
	}
	b, err := yaml.Marshal(&doc)
	if err != nil {
		return source, original, err
	}
	preview, err := workflow.Parse(b)
	if err != nil {
		return source, original, err
	}
	return string(b), preview, nil
}
