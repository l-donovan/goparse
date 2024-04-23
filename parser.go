package goparse

import (
	"fmt"
	"os"
	"slices"
	"strings"
)

type FlagConfig struct {
	LongName     string
	ShortName    rune
	Description  string
	SetByDefault bool
}

type ValueFlagConfig struct {
	LongName    string
	ShortName   rune
	Description string
	ValueName   string
	Default     string
}

type ParameterConfig struct {
	Name        string
	Description string
	Options     []string
	MinCount    int
}

type Parser struct {
	args              []string
	flagArgs          []FlagConfig
	valueFlagArgs     []ValueFlagConfig
	parameterArgs     []ParameterConfig
	listParameter     *ParameterConfig
	subparserArgument string
	subparsers        map[string]Parser
}

func NewParser() Parser {
	p := Parser{}

	return p
}

type SubparserMap map[string]func(parser *Parser)

func (p *Parser) AddFlag(longName string, shortName rune, description string, setByDefault bool) {
	c := FlagConfig{
		LongName:     longName,
		ShortName:    shortName,
		Description:  description,
		SetByDefault: setByDefault,
	}

	p.flagArgs = append(p.flagArgs, c)
}

func (p *Parser) AddValueFlag(longName string, shortName rune, description string, valueName string, defaultValue string) {
	c := ValueFlagConfig{
		LongName:    longName,
		ShortName:   shortName,
		Description: description,
		ValueName:   strings.ToUpper(valueName),
		Default:     defaultValue,
	}

	p.valueFlagArgs = append(p.valueFlagArgs, c)
}

func (p *Parser) AddParameter(name string, description string) {
	c := ParameterConfig{
		Name:        name,
		Description: description,
		Options:     []string{},
	}

	p.parameterArgs = append(p.parameterArgs, c)
}

func (p *Parser) AddChoiceParameter(name string, description string, options []string) {
	c := ParameterConfig{
		Name:        name,
		Description: description,
		Options:     options,
	}

	p.parameterArgs = append(p.parameterArgs, c)
}

func (p *Parser) AddListParameter(name string, description string, min int) error {
	if p.listParameter != nil {
		return fmt.Errorf("parsers support a maximum of one list parameter")
	}

	c := ParameterConfig{
		Name:        name,
		Description: description,
		Options:     []string{},
		MinCount:    min,
	}

	p.listParameter = &c

	return nil
}

func (p *Parser) Subparse(name string, description string, subparserMap SubparserMap) {
	p.subparserArgument = name
	p.subparsers = map[string]Parser{}
	options := []string{}

	for subparserName, initSubparser := range subparserMap {
		subparser := NewParser()
		initSubparser(&subparser)
		p.subparsers[subparserName] = subparser
		options = append(options, subparserName)
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

func (p *Parser) parseArgs(args []string) (map[string]interface{}, []error) {
	values := map[string]interface{}{}
	hasListParameterArg := p.listParameter != nil
	listValues := []string{}
	currentArgPos := 0
	errors := []error{}
	p.args = args

	// Set defaults

	for _, flagConfig := range p.flagArgs {
		values[flagConfig.LongName] = flagConfig.SetByDefault
	}

	for _, flagValueConfig := range p.valueFlagArgs {
		values[flagValueConfig.LongName] = flagValueConfig.Default
	}

	// Populate values

	for {
		arg, exists := p.popArg()

		if !exists {
			break
		}

		isLongFlag := strings.HasPrefix(arg, "--")
		isShortFlag := strings.HasPrefix(arg, "-")
		isParameterArg := currentArgPos < len(p.parameterArgs)

		if isLongFlag {
			longName := strings.TrimPrefix(arg, "--")
			found := false

			if longName == "help" {
				values["help"] = true
				found = true
			} else {
				for _, flagConfig := range p.flagArgs {
					if flagConfig.LongName == longName {
						values[flagConfig.LongName] = !flagConfig.SetByDefault
						found = true
						break
					}
				}

				for _, flagConfig := range p.valueFlagArgs {
					if flagConfig.LongName == longName {
						flagValue, ok := p.popArg()

						if !ok {
							errors = append(errors, fmt.Errorf("missing value for flag `%s'", longName))
						}

						values[flagConfig.LongName] = flagValue
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
					for _, flagConfig := range p.flagArgs {
						if flagConfig.ShortName == shortName {
							values[flagConfig.LongName] = !flagConfig.SetByDefault
							found = true
							break
						}
					}

					for _, flagConfig := range p.valueFlagArgs {
						if flagConfig.ShortName == shortName {
							flagValue, ok := p.popArg()

							if !ok {
								errors = append(errors, fmt.Errorf("missing value for flag `%c'", shortName))
							}

							values[flagConfig.LongName] = flagValue
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
			parameterConfig := p.parameterArgs[currentArgPos]

			// Kick things over to the subparser

			if parameterConfig.Name == p.subparserArgument {
				subparser, ok := p.subparsers[arg]

				if !ok {
					errors = append(errors, fmt.Errorf("bad argument \"%s\" for parameter `%s'", arg, parameterConfig.Name))
				}

				values[parameterConfig.Name] = arg
				result, subparserErrors := subparser.parseArgs(p.args)

				if len(subparserErrors) > 0 {
					errors = append(errors, subparserErrors...)
				}

				for key, val := range result {
					values[key] = val
				}

				return values, errors
			}

			if len(parameterConfig.Options) > 0 && !slices.Contains(parameterConfig.Options, arg) {
				errors = append(errors, fmt.Errorf("bad argument \"%s\" for parameter `%s'", arg, parameterConfig.Name))
			}

			values[parameterConfig.Name] = arg
			currentArgPos += 1
		} else if hasListParameterArg {
			listValues = append(listValues, arg)
		} else {
			errors = append(errors, fmt.Errorf("Yikes!"))
		}
	}

	// Set list parameter arg if applicable

	if hasListParameterArg {
		if len(listValues) < p.listParameter.MinCount {
			errors = append(errors, fmt.Errorf("list parameter `%s' requires at least %d value(s)", p.listParameter.Name, p.listParameter.MinCount))
		}

		values[p.listParameter.Name] = listValues
	}

	// Determine if any args are missing

	missingArgNames := []string{}

	for i := currentArgPos; i < len(p.parameterArgs); i++ {
		missingArgNames = append(missingArgNames, p.parameterArgs[i].Name)
		errors = append(errors, fmt.Errorf("missing required parameter `%s'", p.parameterArgs[i].Name))
	}

	for name, val := range values {
		if name == "help" && val.(bool) {
			return values, nil
		}
	}

	return values, errors
}

func (p *Parser) ParseArgs() (map[string]interface{}, []error) {
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

func (p *Parser) MustParseArgs() map[string]interface{} {
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

	if len(errors) > 0 {
		subparserArg, found := args[p.subparserArgument]

		if found {
			p.printUsage(subparserArg.(string))
		} else {
			p.printUsage("")
		}

		fmt.Fprintln(os.Stderr, "\nencountered errors when parsing arguments:")

		for _, error := range errors {
			fmt.Fprintf(os.Stderr, " %s\n", error)
		}

		os.Exit(1)
	}

	return args
}

func (p *Parser) getParamString(subparserArg string) string {
	usage := ""

	for _, valueFlagArg := range p.valueFlagArgs {
		usage += fmt.Sprintf(" [-%c, --%s %s]", valueFlagArg.ShortName, valueFlagArg.LongName, valueFlagArg.ValueName)
	}

	for _, flagArg := range p.flagArgs {
		usage += fmt.Sprintf(" [-%c, --%s]", flagArg.ShortName, flagArg.LongName)
	}

	for _, parameter := range p.parameterArgs {
		if subparserArg != "" && parameter.Name == p.subparserArgument {
			subparser, _ := p.subparsers[subparserArg]
			usage += fmt.Sprintf(" \033[3m%s\033[23m", subparserArg)
			usage += subparser.getParamString("")
		} else {
			usage += fmt.Sprintf(" %s", parameter.Name)
		}
	}

	if p.listParameter != nil {
		for i := 0; i < p.listParameter.MinCount; i++ {
			usage += fmt.Sprintf(" %s", p.listParameter.Name)
		}

		usage += fmt.Sprintf(" [%s...]", p.listParameter.Name)
	}

	return usage
}

func (p *Parser) PrintUsage() {
	p.printUsage("")
}

func (p *Parser) getFlagDescriptions(subparserArg string) string {
	if subparserArg != "" {
		subparser, _ := p.subparsers[subparserArg]
		return subparser.getFlagDescriptions("")
	}

	usage := ""
	prefixes := []string{}
	descriptions := []string{}
	maxPrefixLen := 0

	for _, valueFlagArg := range p.valueFlagArgs {
		prefix := fmt.Sprintf("-%c, --%s %s", valueFlagArg.ShortName, valueFlagArg.LongName, valueFlagArg.ValueName)
		prefixLen := len(prefix)

		if prefixLen > maxPrefixLen {
			maxPrefixLen = prefixLen
		}

		prefixes = append(prefixes, prefix)
		descriptions = append(descriptions, valueFlagArg.Description)
	}

	for _, flagArg := range p.flagArgs {
		prefix := fmt.Sprintf("-%c, --%s", flagArg.ShortName, flagArg.LongName)
		prefixLen := len(prefix)

		if prefixLen > maxPrefixLen {
			maxPrefixLen = prefixLen
		}

		prefixes = append(prefixes, prefix)
		descriptions = append(descriptions, flagArg.Description)
	}

	for i, prefix := range prefixes {
		usage += fmt.Sprintf("\n %s:%*c%s", prefix, maxPrefixLen-len(prefix)+1, ' ', descriptions[i])
	}

	return usage
}

func (p *Parser) getParameterDescriptions(subparserArg string) string {
	if subparserArg != "" {
		subparser, _ := p.subparsers[subparserArg]
		return subparser.getParameterDescriptions("")
	}

	usage := ""
	prefixes := []string{}
	descriptions := []string{}
	maxPrefixLen := 0

	for _, parameter := range p.parameterArgs {
		prefix := parameter.Name
		prefixLen := len(prefix)

		if prefixLen > maxPrefixLen {
			maxPrefixLen = prefixLen
		}

		prefixes = append(prefixes, prefix)
		descriptions = append(descriptions, parameter.Description)
	}

	if p.listParameter != nil {
		prefix := ""

		for i := 0; i < p.listParameter.MinCount; i++ {
			prefix += fmt.Sprintf(" %s", p.listParameter.Name)
		}

		prefix += fmt.Sprintf(" [%s...]", p.listParameter.Name)
		prefixLen := len(prefix)

		if prefixLen > maxPrefixLen {
			maxPrefixLen = prefixLen
		}

		prefixes = append(prefixes, prefix)
		descriptions = append(descriptions, p.listParameter.Description)
	}

	for i, prefix := range prefixes {
		usage += fmt.Sprintf("\n %s:%*c%s", prefix, maxPrefixLen-len(prefix)+1, ' ', descriptions[i])
	}

	return usage
}

func (p *Parser) getParameterOptions(subparserArg string) []string {
	if subparserArg != "" {
		subparser, _ := p.subparsers[subparserArg]
		return subparser.getParameterOptions("")
	}

	options := []string{}

	for _, parameter := range p.parameterArgs {
		if len(parameter.Options) > 0 && (parameter.Name != p.subparserArgument || subparserArg == "") {
			optionString := fmt.Sprintf("\noptions for parameter `%s':", parameter.Name)

			for _, option := range parameter.Options {
				optionString += "\n " + option
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

	fmt.Fprintf(os.Stderr, "%s\n", usage)
}
