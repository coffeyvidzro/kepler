from pathlib import Path


BRANCH_WORKFLOW = Path(".github/workflows/consolidate-templates-topics.yml")
SCRIPT = Path("scripts/consolidate_templates_topics_modules.py")


def replace_import_block(text: str, imports: str) -> str:
    start = text.index("import (")
    end = text.index(")\n\n", start) + 2
    return text[:start] + imports + text[end:]


def insert_before(text: str, marker: str, content: str) -> str:
    index = text.index(marker)
    return text[:index] + content.rstrip() + "\n\n" + text[index:]


def consolidate_message_templates() -> None:
    root = Path("internal/modules/messagetemplate")
    compatibility_path = root / "compatibility.go"
    compatibility = compatibility_path.read_text()

    model_path = root / "model.go"
    model = model_path.read_text()
    model = replace_import_block(
        model,
        'import (\n\t"encoding/json"\n\t"fmt"\n\t"time"\n)',
    )
    model_types = compatibility[
        compatibility.index("type StringList") : compatibility.index("func (s *Service) CreateAPI")
    ].rstrip()
    model += '''

const (
\tObjectTemplate = "template"
\tObjectList     = "list"
)

''' + model_types + "\n"
    model_path.write_text(model)

    service_path = root / "service.go"
    service = service_path.read_text()

    constants_start = service.index("const (")
    email_sender_start = service.index("type EmailSender")
    validation_declarations = service[constants_start:email_sender_start]
    validation_declarations = validation_declarations.replace(
        "const (\n", "const (\n\tmaxAPIPerPage             = 100\n", 1
    )
    service = service[:constants_start] + service[email_sender_start:]

    validation_start = service.index("func validateCreate")
    require_access_start = service.index("func requireAccess", validation_start)
    validation_core = service[validation_start:require_access_start]
    service = service[:validation_start] + service[require_access_start:]

    api_start = compatibility.index("func (s *Service) CreateAPI")
    api_end = compatibility.index("func normalizeAPIListRequest")
    api_methods = compatibility[api_start:api_end]

    format_start = compatibility.index("func formatSender")
    format_end = compatibility.index("func normalizeReplyTo", format_start)
    format_sender = compatibility[format_start:format_end]

    optional_start = compatibility.index("func optionalNonEmpty")
    optional_non_empty = compatibility[optional_start:]

    service = insert_before(
        service,
        "func (s *Service) Create(ctx context.Context, req CreateRequest)",
        api_methods + "\n" + format_sender + "\n" + optional_non_empty,
    )
    service = replace_import_block(
        service,
        '''import (
\t"context"
\t"errors"
\t"fmt"
\t"net/mail"
\t"slices"
\t"strings"

\t"github.com/google/uuid"
\t"github.com/jackc/pgx/v5"

\temailmodule "github.com/coffeyvidzro/dugble/server/internal/modules/email"
\t"github.com/coffeyvidzro/dugble/server/internal/platform/audit"
\t"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
\tapperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)''',
    )
    service_path.write_text(service)

    normalize_api_start = compatibility.index("func normalizeAPIListRequest")
    split_sender_start = compatibility.index("func splitSender", normalize_api_start)
    format_sender_start = compatibility.index("func formatSender", split_sender_start)
    list_and_sender_validation = compatibility[normalize_api_start:format_sender_start]

    reply_to_start = compatibility.index("func normalizeReplyTo", format_sender_start)
    optional_non_empty_start = compatibility.index("func optionalNonEmpty", reply_to_start)
    reply_to_validation = compatibility[reply_to_start:optional_non_empty_start]

    validation = '''package messagetemplate

import (
\t"encoding/json"
\t"net/mail"
\t"regexp"
\t"strings"
\t"unicode/utf8"

\t"github.com/google/uuid"

\tapperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

''' + validation_declarations + validation_core + list_and_sender_validation + reply_to_validation
    (root / "validation.go").write_text(validation)

    compatibility_path.unlink()
    compatibility_test = root / "compatibility_test.go"
    if compatibility_test.exists():
        compatibility_test.rename(root / "service_test.go")


def consolidate_topics() -> None:
    root = Path("internal/modules/topic")
    compatibility_path = root / "compatibility.go"
    compatibility = compatibility_path.read_text()

    model_path = root / "model.go"
    model = model_path.read_text().rstrip()
    model_types = compatibility[
        compatibility.index("type APIListRequest") : compatibility.index("func (s *Service) CreateAPI")
    ].rstrip()
    model += '''

const (
\tObjectTopic = "topic"
\tObjectList  = "list"
)

''' + model_types + "\n"
    model_path.write_text(model)

    service_path = root / "service.go"
    service = service_path.read_text()

    validation_start = service.index("func validateCreate")
    require_tenant_start = service.index("func requireTenant", validation_start)
    validation_core = service[validation_start:require_tenant_start]

    normalize_list_start = service.index("func normalizeListRequest", require_tenant_start)
    normalize_list = service[normalize_list_start:]
    require_tenant = service[require_tenant_start:normalize_list_start]
    service = service[:validation_start] + require_tenant

    api_start = compatibility.index("func (s *Service) CreateAPI")
    api_end = compatibility.index("func normalizeAPIListRequest")
    api_methods = compatibility[api_start:api_end]
    service = insert_before(
        service,
        "func (s *Service) Create(ctx context.Context, req CreateRequest)",
        api_methods,
    )
    service = replace_import_block(
        service,
        '''import (
\t"context"
\t"errors"
\t"slices"
\t"strings"

\t"github.com/google/uuid"
\t"github.com/jackc/pgx/v5"

\t"github.com/coffeyvidzro/dugble/server/internal/platform/audit"
\t"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
\tapperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)''',
    )
    service_path.write_text(service)

    api_validation = compatibility[compatibility.index("func normalizeAPIListRequest") :]
    validation = '''package topic

import (
\t"strings"

\t"github.com/google/uuid"

\tapperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

const maxAPITopicPage = 100

''' + validation_core + normalize_list + "\n" + api_validation
    (root / "validation.go").write_text(validation)

    compatibility_path.unlink()
    compatibility_test = root / "compatibility_test.go"
    if compatibility_test.exists():
        compatibility_test.rename(root / "service_test.go")


consolidate_message_templates()
consolidate_topics()

if BRANCH_WORKFLOW.exists():
    BRANCH_WORKFLOW.unlink()
if SCRIPT.exists():
    SCRIPT.unlink()
