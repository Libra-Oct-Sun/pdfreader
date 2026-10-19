$ErrorActionPreference = "Stop"

Write-Host "==> Go tests" -ForegroundColor Cyan
go test ./... -count=1

Write-Host "==> Frontend checks" -ForegroundColor Cyan
Push-Location frontend
try {
    npm run check
    npm run build
}
finally {
    Pop-Location
}

Write-Host "All checks passed." -ForegroundColor Green
