$ErrorActionPreference = 'Stop'
chcp 65001 | Out-Null
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
[Console]::InputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8
$root = Split-Path -Parent $PSScriptRoot
$configPath = Join-Path $root 'config\config.yml'
if (-not (Test-Path $configPath)) {
  throw "config.yml not found at $configPath"
}
$config = Get-Content $configPath -Raw
$baseUrl = ([regex]::Match($config, 'baseURL:\s*"([^"]+)"')).Groups[1].Value
$apiKey = ([regex]::Match($config, 'apiKey:\s*"([^"]+)"')).Groups[1].Value
$model = ([regex]::Match($config, 'model:\s*"([^"]+)"')).Groups[1].Value
if (-not $baseUrl) { throw 'baseURL not found in config.yml' }
if (-not $apiKey) { throw 'apiKey not found in config.yml' }
if (-not $model) { throw 'model not found in config.yml' }

$uri = $baseUrl.TrimEnd('/') + '/chat/completions'
$body = @{
  model = $model
  messages = @(
    @{ role = 'system'; content = '请用中文简短回答：1+1=? 仅输出结果。' }
  )
  temperature = 0
} | ConvertTo-Json -Depth 6

Write-Host "Testing: $uri"
try {
  $response = Invoke-RestMethod -Method Post -Uri $uri -Headers @{
    Authorization = "Bearer $apiKey"
    'Content-Type' = 'application/json'
  } -Body $body
  $content = $response.choices[0].message.content
  $gbk = [System.Text.Encoding]::GetEncoding(936)
  $fixed = [System.Text.Encoding]::UTF8.GetString($gbk.GetBytes($content))
  Write-Host "OK: $fixed"
} catch {
  if ($_.Exception.Response -and $_.Exception.Response.GetResponseStream()) {
    $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
    $errBody = $reader.ReadToEnd()
    Write-Host "Error body: $errBody"
  }
  throw
}
