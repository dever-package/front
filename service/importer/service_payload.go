package importer

type ServiceFieldPayload struct {
	Field         string   `json:"field"`
	Label         string   `json:"label"`
	Kind          string   `json:"kind"`
	Use           string   `json:"use"`
	Multiple      bool     `json:"multiple"`
	MissingPolicy string   `json:"missingPolicy"`
	SaveMode      string   `json:"saveMode"`
	UploadKind    string   `json:"uploadKind"`
	UploadRuleID  int      `json:"uploadRuleId"`
	SourceMode    string   `json:"sourceMode"`
	BaseDir       string   `json:"baseDir"`
	Delimiters    []string `json:"delimiters"`
	ParentField   string   `json:"parentField"`
	RootValue     any      `json:"rootValue"`
}

type ServiceRelationPayload struct {
	Field            string `json:"field"`
	Option           string `json:"option"`
	Mode             string `json:"mode"`
	OwnerField       string `json:"ownerField"`
	TargetField      string `json:"targetField"`
	OptionValueField string `json:"optionValueField"`
	OptionLabelField string `json:"optionLabelField"`
}

type ServicePayload struct {
	Field    ServiceFieldPayload    `json:"field"`
	Relation ServiceRelationPayload `json:"relation"`
	Value    any                    `json:"value"`
}

func BuildServicePayload(field ServiceFieldPayload, relation ServiceRelationPayload, value any) ServicePayload {
	return ServicePayload{
		Field:    field,
		Relation: relation,
		Value:    value,
	}
}

func ParseServicePayload(params []any) ServicePayload {
	if len(params) == 0 {
		return ServicePayload{}
	}

	raw, _ := params[0].(map[string]any)
	if raw == nil {
		return ServicePayload{}
	}

	var payload ServicePayload
	_ = parseJSONValue(raw, &payload)
	return payload
}

func WrapServiceValue(value any) map[string]any {
	return map[string]any{
		"value": value,
	}
}

func UnwrapServiceValue(result any) any {
	payload, ok := result.(map[string]any)
	if !ok {
		return result
	}
	if value, exists := payload["value"]; exists {
		return value
	}
	return result
}
