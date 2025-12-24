{{- /* Heavily inspired by the Go toolchain and fd */ -}}
{{prettyHeader "Usage"}}
{{indent .FullUsage 2}}


{{ with .Short }}
{{- indent . 2 | wrapTTY }}
{{"\n"}}
{{- end}}

{{- with .Deprecated }}
{{- indent (printf "DEPRECATED: %s" .) 2 | wrapTTY }}
{{"\n"}}
{{- end }}

{{ with .Aliases }}
{{"  Aliases: "}} {{- joinStrings .}}
{{- end }}

{{- with .Long}}
{{"\n"}}
{{- indent . 2}}
{{ "\n" }}
{{- end }}
{{ with visibleChildren . }}
{{- range $index, $child := . }}
{{- if eq $index 0 }}
{{ prettyHeader "Subcommands"}}
{{- end }}
    {{- "\n" }}
    {{- formatSubcommand . | trimNewline }}
{{- end }}
{{- "\n" }}
{{- end }}
{{- $groups := optionGroups . }}
{{- if gt (len $groups) 0 }}
{{- range $index, $group := $groups }}
{{ prettyHeader (printf "%s Options" $group.Name) }}
    {{- range $optIndex, $option := $group.Options }}
	{{- if not (eq $option.Shorthand "") }}{{- print "\n "}} {{ keyword "-"}}{{keyword $option.Shorthand }}{{", "}}
	{{- else }}{{- print "\n      " -}}
	{{- end }}
    {{- with flagName $option }}{{keyword "--"}}{{ keyword . }}{{ end }} {{- with typeHelper $option }} {{ . }}{{ end }}
    {{- with envName $option }}, {{ . | keyword }}{{ end }}
    {{- if or $option.Default $option.Required }}{{- " (" -}}
      {{- with $option.Default }}default: {{ . }}{{ end }}
      {{- if and $option.Default $option.Required }}, {{ end }}
      {{- if $option.Required }}required{{ end }}
    {{- ")" -}}{{- end }}
        {{- with $option.Description }}
            {{- $desc := $option.Description }}
{{ indent $desc 10 }}
{{- if isDeprecated $option }}
{{- if $option.Deprecated }}
{{ indent (printf "DEPRECATED: %s" $option.Deprecated) 10 }}
{{- else }}
{{ indent "DEPRECATED: This option is deprecated." 10 }}
{{- end }}
{{- end }}
        {{- end -}}
    {{- end }}
{{- end }}
{{- end }}
{{- if hasParent . }}
———
Run `{{ rootCommandName . }} --help` for a list of global options.
{{- else }}
{{- end }}