package goparse

import (
	"fmt"
	"os"
	"strings"

	"al.essio.dev/pkg/shellescape"
)

type flagParam struct {
	longName     string
	shortName    rune
	description  string
	setByDefault bool
}

type valueFlagParam struct {
	longName     string
	shortName    rune
	description  string
	valueName    string
	defaultValue string
}

type paramOption struct {
	name   string
	hidden bool
}

type positionalParam struct {
	name        string
	description string
	options     []paramOption
	minCount    int
}

type Parser struct {
	args              []string
	flagParams        []flagParam
	valueFlagParams   []valueFlagParam
	positionalParams  []positionalParam
	listParam         *positionalParam
	subparserArgument string
	subparsers        map[string]Parser
}

func NewParser() Parser {
	p := Parser{}

	return p
}

// SubparserMap is a map of parameter values to functions that accept a pointer to a Parser struct
// in which subparser behavior can be configured.
type SubparserMap map[string]func(subparser *Parser)

// marshal does the heavy lifting for Marshal, iterating through the positional, flag, value flag,
// and list parameters in a parser to turn a map of input values into a command string.
//
// marshal differs from Marshal in that it does not parse the optional subparser argument.
// This avoids endless recursion.
func (p *Parser) marshal(values map[string]any) ([]string, error) {
	var arguments []string

	// Handle positional parameters.

	for _, param := range p.positionalParams {
		value, exists := values[param.name]

		if !exists {
			return nil, fmt.Errorf("missing required positional parameter `%s'", param.name)
		}

		val := fmt.Sprintf("%s", value)
		arguments = append(arguments, shellescape.Quote(val))
	}

	// Handle value flag parameters.

	for _, param := range p.valueFlagParams {
		value, exists := values[param.longName]

		if exists {
			val := fmt.Sprintf("%s", value)
			arguments = append(arguments, "--"+param.longName, shellescape.Quote(val))
		} else {
			arguments = append(arguments, "--"+param.longName, shellescape.Quote(param.defaultValue))
		}
	}

	// Handle flag parameters.

	for _, param := range p.flagParams {
		value, exists := values[param.longName]

		if exists {
			isSet, ok := value.(bool)

			if !ok {
				return nil, fmt.Errorf("expected value of type bool for flag parameter `%s' but found %T", param.longName, value)
			}

			if isSet {
				arguments = append(arguments, "--"+param.longName)
			}
		} else if param.setByDefault {
			arguments = append(arguments, "--"+param.longName)
		}
	}

	// Handle the optional list parameter.

	if p.listParam != nil {
		value, exists := values[p.listParam.name]

		if !exists && p.listParam.minCount > 0 {
			return nil, fmt.Errorf("list parameter requires at least %d value(s) but found none", p.listParam.minCount)
		}

		switch val := value.(type) {
		case []string:
			if len(val) < p.listParam.minCount {
				return nil, fmt.Errorf("list parameter requires at least %d value(s) but found %d", p.listParam.minCount, len(val))
			}

			valueStrings := make([]string, len(val))

			for i, value := range val {
				valueStrings[i] = shellescape.Quote(value)
			}

			arguments = append(arguments, valueStrings...)
		case []any:
			if len(val) < p.listParam.minCount {
				return nil, fmt.Errorf("list parameter requires at least %d value(s) but found %d", p.listParam.minCount, len(val))
			}

			valueStrings := make([]string, len(val))

			for i, value := range val {
				valueStrings[i] = shellescape.Quote(fmt.Sprintf("%s", value))
			}

			arguments = append(arguments, valueStrings...)
		default:
			return nil, fmt.Errorf("expected value of type []any or []string for list parameter `%s' but found %T", p.listParam.name, value)
		}
	}

	return arguments, nil
}

// Marshal turns a map of input values into a corresponding command string. It is the functional
// inverse of ParseArgs, which turns a command string into a map of input values.
func (p *Parser) Marshal(executable string, values map[string]any) (string, error) {
	var subp *Parser
	var arguments []string

	// Check if the executable was provided.

	if executable != "" {
		arguments = append(arguments, executable)
	}

	// Check if we have a subparser.

	if p.subparserArgument != "" {
		subparserName, exists := values[p.subparserArgument]

		if !exists {
			return "", fmt.Errorf("subparser argument `%s' was not provided", p.subparserArgument)
		}

		subparser, exists := p.subparsers[subparserName.(string)]

		if !exists {
			return "", fmt.Errorf("subparser `%s' for argument `%s' does not exist", subparserName, p.subparserArgument)
		}

		subp = &subparser
	}

	// Marshal parser arguments.

	args, err := p.marshal(values)

	if err != nil {
		return "", fmt.Errorf("marshal arguments: %w", err)
	}

	arguments = append(arguments, args...)

	// Marshal subparser arguments.

	if subp != nil {
		args, err := subp.marshal(values)

		if err != nil {
			return "", fmt.Errorf("marshal subparser arguments: %w", err)
		}

		arguments = append(arguments, args...)
	}

	return strings.Join(arguments, " "), nil
}

func (p *Parser) AddFlag(longName string, shortName rune, description string, setByDefault bool) {
	c := flagParam{
		longName:     longName,
		shortName:    shortName,
		description:  description,
		setByDefault: setByDefault,
	}

	p.flagParams = append(p.flagParams, c)
}

func (p *Parser) AddValueFlag(longName string, shortName rune, description string, valueName string, defaultValue string) {
	c := valueFlagParam{
		longName:     longName,
		shortName:    shortName,
		description:  description,
		valueName:    strings.ToUpper(valueName),
		defaultValue: defaultValue,
	}

	p.valueFlagParams = append(p.valueFlagParams, c)
}

func (p *Parser) AddParameter(name string, description string) {
	c := positionalParam{
		name:        name,
		description: description,
		options:     []paramOption{},
	}

	p.positionalParams = append(p.positionalParams, c)
}

func (p *Parser) AddChoiceParameter(name string, description string, options []paramOption) {
	c := positionalParam{
		name:        name,
		description: description,
		options:     options,
	}

	p.positionalParams = append(p.positionalParams, c)
}

// SetListParameter sets the list parameter on the parser.
func (p *Parser) SetListParameter(name string, description string, min int) {
	c := positionalParam{
		name:        name,
		description: description,
		options:     []paramOption{},
		minCount:    min,
	}

	p.listParam = &c
}

// AddListParameter adds a list parameter to the parser if one doesn't already exist.
// If it does, an error is returned.
//
// Deprecated: Use SetListParameter instead to avoid unnecessary error handling.
func (p *Parser) AddListParameter(name string, description string, min int) error {
	if p.listParam != nil {
		return fmt.Errorf("parsers support a maximum of one list parameter")
	}

	c := positionalParam{
		name:        name,
		description: description,
		options:     []paramOption{},
		minCount:    min,
	}

	p.listParam = &c

	return nil
}

// Subparse configures a subparser, adding a parameter that defers to a secondary parser
// chosen by its value.
func (p *Parser) Subparse(name string, description string, subparserMap SubparserMap) {
	p.subparserArgument = name
	p.subparsers = map[string]Parser{}
	var options []paramOption

	for subparserName, initSubparser := range subparserMap {
		option := paramOption{
			name: subparserName,
		}

		if option.name[0] == '_' {
			option.hidden = true
			option.name = option.name[1:]
		}

		subparser := NewParser()
		initSubparser(&subparser)
		p.subparsers[option.name] = subparser
		options = append(options, option)
	}

	p.AddChoiceParameter(name, description, options)
}

func (p *Parser) popArg() (string, bool) {
	if len(p.args) == 0 {
		return "", false
	}

	val := p.args[0]
	p.args = p.args[1:]

	return val, true
}

func (p *Parser) parseArgs(args []string) (map[string]any, []error) {
	values := map[string]any{}
	hasListParameterArg := p.listParam != nil
	var listValues []string
	currentArgPos := 0
	var errors []error
	p.args = args

	// Set defaults

	for _, flagConfig := range p.flagParams {
		values[flagConfig.longName] = flagConfig.setByDefault
	}

	for _, flagValueConfig := range p.valueFlagParams {
		values[flagValueConfig.longName] = flagValueConfig.defaultValue
	}

	// Populate values

	for {
		arg, exists := p.popArg()

		if !exists {
			break
		}

		isLongFlag := strings.HasPrefix(arg, "--")
		isShortFlag := strings.HasPrefix(arg, "-")
		isParameterArg := currentArgPos < len(p.positionalParams)

		if isLongFlag {
			longName := strings.TrimPrefix(arg, "--")
			found := false

			if longName == "help" {
				values["help"] = true
				found = true
			} else {
				for _, flagConfig := range p.flagParams {
					if flagConfig.longName == longName {
						values[flagConfig.longName] = !flagConfig.setByDefault
						found = true
						break
					}
				}

				for _, flagConfig := range p.valueFlagParams {
					if flagConfig.longName == longName {
						flagValue, ok := p.popArg()

						if !ok {
							errors = append(errors, fmt.Errorf("missing value for flag `%s'", longName))
						}

						values[flagConfig.longName] = flagValue
						found = true
					}
				}
			}

			if !found {
				errors = append(errors, fmt.Errorf("unknown flag `--%s'", longName))
			}

			continue
		} else if isShortFlag {
			flags := strings.TrimPrefix(arg, "-")

			for _, shortName := range flags {
				found := false

				if shortName == 'h' {
					values["help"] = true
					found = true
				} else {
					for _, flagConfig := range p.flagParams {
						if flagConfig.shortName == shortName {
							values[flagConfig.longName] = !flagConfig.setByDefault
							found = true
							break
						}
					}

					for _, flagConfig := range p.valueFlagParams {
						if flagConfig.shortName == shortName {
							flagValue, ok := p.popArg()

							if !ok {
								errors = append(errors, fmt.Errorf("missing value for flag `%c'", shortName))
							}

							values[flagConfig.longName] = flagValue
							found = true
							break
						}
					}
				}

				if !found {
					errors = append(errors, fmt.Errorf("unknown flag `-%c'", shortName))
				}
			}

			continue
		} else if isParameterArg {
			parameterConfig := p.positionalParams[currentArgPos]

			// Kick things over to the subparser

			if parameterConfig.name == p.subparserArgument {
				subparser, ok := p.subparsers[arg]

				if !ok {
					errors = append(errors, fmt.Errorf("bad argument \"%s\" for parameter `%s'", arg, parameterConfig.name))
				}

				values[parameterConfig.name] = arg
				result, subparserErrors := subparser.parseArgs(p.args)

				if len(subparserErrors) > 0 {
					errors = append(errors, subparserErrors...)
				}

				for key, val := range result {
					values[key] = val
				}

				return values, errors
			}

			for _, option := range parameterConfig.options {
				if option.name == arg {
					errors = append(errors, fmt.Errorf("bad argument \"%s\" for parameter `%s'", arg, parameterConfig.name))
					break
				}
			}

			values[parameterConfig.name] = arg
			currentArgPos += 1
		} else if hasListParameterArg {
			listValues = append(listValues, arg)
		} else {
			errors = append(errors, fmt.Errorf("received unexpected argument \"%s\"", arg))
		}
	}

	// Set list parameter arg if applicable

	if hasListParameterArg {
		if len(listValues) < p.listParam.minCount {
			errors = append(errors, fmt.Errorf("list parameter `%s' requires at least %d value(s)", p.listParam.name, p.listParam.minCount))
		}

		values[p.listParam.name] = listValues
	}

	// Determine if any args are missing

	for i := currentArgPos; i < len(p.positionalParams); i++ {
		errors = append(errors, fmt.Errorf("missing required parameter `%s'", p.positionalParams[i].name))
	}

	for name, val := range values {
		if name == "help" && val.(bool) {
			return values, nil
		}
	}

	return values, errors
}

func (p *Parser) ParseArgs() (map[string]any, []error) {
	osArgs := os.Args[1:]
	args, errors := p.parseArgs(osArgs)

	if _, ok := args["help"]; ok {
		subparserArg, found := args[p.subparserArgument]

		if found {
			p.printUsage(subparserArg.(string))
		} else {
			p.printUsage("")
		}

		os.Exit(0)
	}

	return args, errors
}

func (p *Parser) MustParseArgs() map[string]any {
	args, errors := p.ParseArgs()

	if len(errors) > 0 {
		subparserArg, found := args[p.subparserArgument]

		if found {
			p.printUsage(subparserArg.(string))
		} else {
			p.printUsage("")
		}

		_, _ = fmt.Fprintln(os.Stderr, "\nencountered errors when parsing arguments:")

		for _, err := range errors {
			_, _ = fmt.Fprintf(os.Stderr, " %s\n", err)
		}

		os.Exit(1)
	}

	return args
}

func (p *Parser) getParamString(subparserArg string) string {
	usage := ""

	for _, valueFlagArg := range p.valueFlagParams {
		usage += fmt.Sprintf(" [-%c, --%s %s]", valueFlagArg.shortName, valueFlagArg.longName, valueFlagArg.valueName)
	}

	for _, flagArg := range p.flagParams {
		usage += fmt.Sprintf(" [-%c, --%s]", flagArg.shortName, flagArg.longName)
	}

	for _, parameter := range p.positionalParams {
		if subparserArg != "" && parameter.name == p.subparserArgument {
			subparser := p.subparsers[subparserArg]
			usage += fmt.Sprintf(" \033[3m%s\033[23m", subparserArg)
			usage += subparser.getParamString("")
		} else {
			usage += fmt.Sprintf(" %s", parameter.name)
		}
	}

	if p.listParam != nil {
		for i := 0; i < p.listParam.minCount; i++ {
			usage += fmt.Sprintf(" %s", p.listParam.name)
		}

		usage += fmt.Sprintf(" [%s...]", p.listParam.name)
	}

	return usage
}

func (p *Parser) PrintUsage() {
	p.printUsage("")
}

func (p *Parser) getFlagDescriptions(subparserArg string) string {
	if subparserArg != "" {
		subparser := p.subparsers[subparserArg]
		return subparser.getFlagDescriptions("")
	}

	usage := ""
	prefixes := make([]string, len(p.valueFlagParams)+len(p.flagParams))
	maxPrefixLen := 0

	for i, valueFlagArg := range p.valueFlagParams {
		prefix := fmt.Sprintf("-%c, --%s %s", valueFlagArg.shortName, valueFlagArg.longName, valueFlagArg.valueName)
		prefixLen := len(prefix)

		if prefixLen > maxPrefixLen {
			maxPrefixLen = prefixLen
		}

		prefixes[i] = prefix
	}

	for i, flagArg := range p.flagParams {
		prefix := fmt.Sprintf("-%c, --%s", flagArg.shortName, flagArg.longName)
		prefixLen := len(prefix)

		if prefixLen > maxPrefixLen {
			maxPrefixLen = prefixLen
		}

		prefixes[i+len(p.valueFlagParams)] = prefix
	}

	for i, prefix := range prefixes {
		usage += fmt.Sprintf("\n %s:%*c", prefix, maxPrefixLen-len(prefix)+1, ' ')

		if i < len(p.valueFlagParams) {
			usage += fmt.Sprintf("%s (default \"%s\")", p.valueFlagParams[i].description, p.valueFlagParams[i].defaultValue)
		} else {
			usage += p.flagParams[i-len(p.valueFlagParams)].description
		}
	}

	return usage
}

func (p *Parser) getParameterDescriptions(subparserArg string) string {
	if subparserArg != "" {
		subparser := p.subparsers[subparserArg]
		return subparser.getParameterDescriptions("")
	}

	usage := ""
	var prefixes []string
	var descriptions []string
	maxPrefixLen := 0

	for _, parameter := range p.positionalParams {
		prefix := parameter.name
		prefixLen := len(prefix)

		if prefixLen > maxPrefixLen {
			maxPrefixLen = prefixLen
		}

		prefixes = append(prefixes, prefix)
		descriptions = append(descriptions, parameter.description)
	}

	if p.listParam != nil {
		prefix := ""

		for i := 0; i < p.listParam.minCount; i++ {
			prefix += fmt.Sprintf(" %s", p.listParam.name)
		}

		prefix += fmt.Sprintf(" [%s...]", p.listParam.name)
		prefixLen := len(prefix)

		if prefixLen > maxPrefixLen {
			maxPrefixLen = prefixLen
		}

		prefixes = append(prefixes, prefix)
		descriptions = append(descriptions, p.listParam.description)
	}

	for i, prefix := range prefixes {
		usage += fmt.Sprintf("\n %s:%*c%s", prefix, maxPrefixLen-len(prefix)+1, ' ', descriptions[i])
	}

	return usage
}

func (p *Parser) getParameterOptions(subparserArg string) []string {
	if subparserArg != "" {
		subparser := p.subparsers[subparserArg]
		return subparser.getParameterOptions("")
	}

	var options []string

	for _, parameter := range p.positionalParams {
		if len(parameter.options) > 0 {
			optionString := fmt.Sprintf("\noptions for parameter `%s':", parameter.name)

			for _, option := range parameter.options {
				if !option.hidden {
					optionString += "\n " + option.name
				}
			}

			options = append(options, optionString)
		}
	}

	return options
}

func (p *Parser) printUsage(subparserArg string) {
	usage := fmt.Sprintf("usage: %s", os.Args[0])
	usage += p.getParamString(subparserArg)

	flagDescription := p.getFlagDescriptions(subparserArg)

	if len(flagDescription) > 0 {
		usage += "\n\nflags:"
		usage += flagDescription
	}

	paramDescription := p.getParameterDescriptions(subparserArg)

	if len(paramDescription) > 0 {
		usage += "\n\nparameters:"
		usage += paramDescription
	}

	optionDescriptions := p.getParameterOptions(subparserArg)

	for _, optionDescription := range optionDescriptions {
		usage += "\n" + optionDescription
	}

	_, _ = fmt.Fprintln(os.Stderr, usage)
}
