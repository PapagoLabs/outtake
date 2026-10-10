{{- .Title | replaceRE "\n" " " | printf "# %s" }}

{{ .Description }}.
{{ with site.GetPage "/setup/install" }}
[Get Started]({{ .Permalink }})
{{ end }}
{{- range .Site.Sections }}
- [{{ .Title }}]({{ .Permalink }}){{ with .Description }}: {{ . }}{{ end }}
{{- end }}
