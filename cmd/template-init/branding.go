package main

import (
	"encoding/json"
	"fmt"
	"go/scanner"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var jsonString = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)

func replaceBrandedSource(path, input string, replace, dynamic *strings.Replacer) (string, error) {
	switch filepath.Ext(path) {
	case ".go":
		return replaceGoBranding(input, replace)
	case ".json":
		if !json.Valid([]byte(input)) {
			if dynamic.Replace(input) != input {
				return "", fmt.Errorf("unsupported dynamic branding in invalid JSON %s", path)
			}
			return replace.Replace(input), nil
		}
		return jsonString.ReplaceAllStringFunc(input, func(token string) string {
			var value string
			if json.Unmarshal([]byte(token), &value) != nil {
				return token
			}
			next := replace.Replace(value)
			if strings.HasPrefix(path, "internal/i18n/locales/") {
				next = replaceLocaleBranding(value, replace)
			}
			if next != value {
				encoded, _ := json.Marshal(next)
				return string(encoded)
			}
			return token
		}), nil
	case ".md":
		return replace.Replace(input), nil
	}
	switch path {
	case "LICENSE":
		return replace.Replace(input), nil
	case "Dockerfile":
		return replaceMetadataLine(input, "LABEL org.opencontainers.image.title=", `"Dynamis Code Apps Template"`, replace, dynamic, false)
	case ".gitleaks.toml":
		return replaceMetadataLine(input, "title = ", `"Dynamis Code secret scanning"`, replace, dynamic, false)
	case ".env.example":
		return replaceMetadataLine(input, "WEBAUTHN_RP_DISPLAY_NAME=", "Dynamis Code", replace, dynamic, true)
	case ".github/ISSUE_TEMPLATE/config.yml":
		return replaceMetadataLine(input, "    url: ", templateSecurityURL, replace, dynamic, false)
	case ".github/ISSUE_TEMPLATE/feature_request.yml":
		// Branding here is within the Markdown body of an issue form.
		return replace.Replace(input), nil
	}
	if dynamic.Replace(input) != input {
		return "", fmt.Errorf("unsupported dynamic branding context in %s", path)
	}
	return replace.Replace(input), nil
}

func replaceLocaleBranding(value string, replace *strings.Replacer) string {
	const marker = "Dynamis Code"
	var output strings.Builder
	for {
		before, after, found := strings.Cut(value, marker)
		output.WriteString(replace.Replace(before))
		if !found {
			return output.String()
		}
		brand := replace.Replace(marker)
		if strings.ContainsAny(brand, "{}") {
			output.WriteString("{{" + strconv.Quote(brand) + "}}")
		} else {
			output.WriteString(brand)
		}
		value = after
	}
}

func replaceMetadataLine(input, prefix, old string, replace, dynamic *strings.Replacer, shell bool) (string, error) {
	lines := strings.Split(input, "\n")
	for i, line := range lines {
		if dynamic.Replace(line) == line {
			lines[i] = replace.Replace(line)
			continue
		}
		if !strings.HasPrefix(line, prefix+old) {
			return "", fmt.Errorf("unsupported metadata substitution outside %s", prefix)
		}
		value := replace.Replace(strings.Trim(old, `"`))
		right := strings.TrimPrefix(line, prefix+old)
		if dynamic.Replace(right) != right {
			return "", fmt.Errorf("unsupported trailing metadata substitution after %s", prefix)
		}
		if shell {
			lines[i] = prefix + "'" + strings.ReplaceAll(value, "'", `'\''`) + "'" + replace.Replace(right)
		} else {
			quoted, _ := json.Marshal(value)
			lines[i] = prefix + string(quoted) + replace.Replace(right)
		}
	}
	return strings.Join(lines, "\n"), nil
}

// Interpret Go string values before replacing identity metadata, so quotes,
// backslashes and source-like input remain data rather than Go tokens.
func replaceGoBranding(input string, replace *strings.Replacer) (string, error) {
	set := token.NewFileSet()
	file := set.AddFile("", -1, len(input))
	var lexer scanner.Scanner
	var scanErr error
	lexer.Init(file, []byte(input), func(position token.Position, message string) {
		if scanErr == nil {
			scanErr = fmt.Errorf("Go source at %s: %s", position, message)
		}
	}, scanner.ScanComments)
	var output strings.Builder
	previous := 0
	for {
		pos, kind, literal := lexer.Scan()
		if kind == token.EOF {
			break
		}
		if kind != token.STRING && kind != token.COMMENT {
			continue
		}
		offset := file.Offset(pos)
		if offset+len(literal) > len(input) || input[offset:offset+len(literal)] != literal {
			return "", fmt.Errorf("unsupported Go branding token offsets")
		}
		output.WriteString(input[previous:offset])
		next := literal
		if kind == token.STRING {
			value, err := strconv.Unquote(literal)
			if err != nil {
				return "", err
			}
			if branded := replace.Replace(value); branded != value {
				next = strconv.Quote(branded)
			}
		} else if branded := replace.Replace(literal); branded != literal {
			if strings.HasPrefix(literal, "//go:") || strings.HasPrefix(literal, "//line ") ||
				strings.HasPrefix(literal, "/*") && strings.Contains(branded[2:len(branded)-2], "*/") {
				return "", fmt.Errorf("unsupported dynamic Go directive or comment branding")
			}
			next = branded
		}
		output.WriteString(next)
		previous = offset + len(literal)
	}
	if scanErr != nil {
		return "", scanErr
	}
	output.WriteString(input[previous:])
	return output.String(), nil
}
