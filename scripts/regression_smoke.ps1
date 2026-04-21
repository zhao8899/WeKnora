param(
    [string]$FrontendDir = "frontend",
    [string]$FrontendUrl = "http://localhost:5173",
    [string]$BackendUrl = "http://localhost:8080",
    [switch]$SkipFrontendRuntime
)

$ErrorActionPreference = "Stop"

function Write-Step {
    param([string]$Message)
    Write-Host ""
    Write-Host "==> $Message" -ForegroundColor Cyan
}

function Invoke-Check {
    param(
        [string]$Name,
        [scriptblock]$Action
    )

    Write-Host " - $Name ... " -NoNewline
    try {
        & $Action
        Write-Host "OK" -ForegroundColor Green
    }
    catch {
        Write-Host "FAILED" -ForegroundColor Red
        throw
    }
}

function Get-HttpStatus {
    param(
        [string]$Url,
        [hashtable]$Headers = @{}
    )

    try {
        $response = Invoke-WebRequest -UseBasicParsing -Uri $Url -Headers $Headers
        return [int]$response.StatusCode
    }
    catch {
        if ($_.Exception.Response) {
            return [int]$_.Exception.Response.StatusCode.value__
        }
        throw
    }
}

function Get-HttpJson {
    param([string]$Url)
    $response = Invoke-WebRequest -UseBasicParsing -Uri $Url
    return $response.Content | ConvertFrom-Json
}

Write-Step "Frontend Static Validation"
Invoke-Check "frontend type-check" {
    Push-Location $FrontendDir
    try {
        npm run type-check | Out-Host
        if ($LASTEXITCODE -ne 0) {
            throw "frontend type-check failed"
        }
    }
    finally {
        Pop-Location
    }
}

Invoke-Check "frontend build" {
    Push-Location $FrontendDir
    try {
        npm run build | Out-Host
        if ($LASTEXITCODE -ne 0) {
            throw "frontend build failed"
        }
    }
    finally {
        Pop-Location
    }
}

Write-Step "Runtime Health"
if (-not $SkipFrontendRuntime) {
    Invoke-Check "frontend dev server reachable" {
        $status = Get-HttpStatus -Url $FrontendUrl
        if ($status -lt 200 -or $status -ge 500) {
            throw "frontend returned unexpected status $status"
        }
    }
}

Invoke-Check "backend /health" {
    $health = Get-HttpJson -Url "$BackendUrl/health"
    if ($health.status -ne "ok") {
        throw "backend health payload is not ok"
    }
}

Invoke-Check "docker app healthy" {
    $status = docker inspect -f "{{.State.Health.Status}}" WeKnora-app
    if ($LASTEXITCODE -ne 0) {
        throw "docker inspect failed"
    }
    if (($status | Out-String).Trim() -ne "healthy") {
        throw "WeKnora-app is not healthy"
    }
}

Write-Step "Core Auth Boundaries"
Invoke-Check "answer confidence requires auth" {
    $status = Get-HttpStatus -Url "$BackendUrl/api/v1/chat/answer/test/confidence"
    if ($status -ne 401) {
        throw "expected 401, got $status"
    }
}

Invoke-Check "analytics requires auth" {
    $status = Get-HttpStatus -Url "$BackendUrl/api/v1/analytics/hot-questions"
    if ($status -ne 401) {
        throw "expected 401, got $status"
    }
}

Invoke-Check "datasource types requires auth" {
    $status = Get-HttpStatus -Url "$BackendUrl/api/v1/datasource/types"
    if ($status -ne 401) {
        throw "expected 401, got $status"
    }
}

Write-Host ""
Write-Host "Regression smoke checks passed." -ForegroundColor Green
