from pathlib import Path

path = Path("internal/modules/sms/repository.go")
source = path.read_text()
old = "dbsqlc.FindApprovedSMSSenderParams{TeamID: &teamID, Name: name}"
new = "dbsqlc.FindApprovedSMSSenderParams{TeamID: teamID, Name: name}"
if source.count(old) != 1:
    raise SystemExit("approved SMS sender callsite was not found")
path.write_text(source.replace(old, new, 1))
