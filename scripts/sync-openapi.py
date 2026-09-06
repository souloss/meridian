#!/usr/bin/env python3
"""Compile the TypeSpec API sources and materialize OpenAPI projections."""

from __future__ import annotations

import argparse
import difflib
import subprocess
import sys
import tempfile
from copy import deepcopy
from pathlib import Path
from typing import Any

import yaml


REPOSITORY_ROOT = Path(__file__).resolve().parents[1]
API_ROOT = REPOSITORY_ROOT / "contracts" / "api"
BUILD_ROOT = REPOSITORY_ROOT / "build" / "contracts" / "api"
CANONICAL = REPOSITORY_ROOT / "contracts" / "openapi.yaml"
OPENAPI_OUTPUT = Path("@typespec") / "openapi3" / "openapi.yaml"
DOMAINS_ROOT = API_ROOT / "domains"
DEFAULT_SECURITY = [{"cookieSession": []}, {"patBearer": []}]
SECURITY_NAMES = {"ApiKeyAuth": "cookieSession", "BearerAuth": "patBearer"}
METHODS = {"get", "put", "post", "delete", "options", "head", "patch", "trace"}


class ContractError(Exception):
    """A source contract error with an actionable message."""


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="fail when the canonical projection is stale")
    args = parser.parse_args()

    try:
        ensure_dependencies()
        with tempfile.TemporaryDirectory(prefix="meridian-tsp-") as temporary:
            temporary_root = Path(temporary)
            common = compile_entry(API_ROOT / "common" / "models.tsp", temporary_root / "common")
            domains = {
                domain.stem: compile_entry(domain, temporary_root / "domains" / domain.stem)
                for domain in sorted(DOMAINS_ROOT.glob("*.tsp"))
            }
            if not domains:
                raise ContractError(f"no TypeSpec domain files found in {DOMAINS_ROOT}")
            main_document = compile_entry(API_ROOT / "main.tsp", temporary_root / "main")

            common_document = normalize_common(common, domains.values())
            domain_documents = {
                name: normalize_domain(name, document, common_document)
                for name, document in domains.items()
            }
            canonical_document = normalize_canonical(main_document)
            validate_documents(canonical_document, domain_documents, common_document)

            generated = {CANONICAL: dump_yaml(canonical_document)}
            generated[BUILD_ROOT / "common" / "openapi.yaml"] = dump_yaml(common_document)
            generated.update(
                {
                    BUILD_ROOT / "domains" / f"{name}.yaml": dump_yaml(document)
                    for name, document in domain_documents.items()
                }
            )

            if args.check:
                return check_outputs(generated)
            write_outputs(generated)
            return 0
    except ContractError as error:
        print(f"contracts sync: {error}", file=sys.stderr)
        return 1


def ensure_dependencies() -> None:
    if (API_ROOT / "node_modules" / ".bin" / "tsp").is_file():
        return
    run(
        [
            "vfox",
            "exec",
            "nodejs@24.20.0",
            "--",
            "pnpm",
            "--dir",
            str(API_ROOT),
            "install",
            "--frozen-lockfile",
            "--ignore-scripts",
        ],
        REPOSITORY_ROOT,
    )


def compile_entry(entry: Path, output_directory: Path) -> dict[str, Any]:
    output_directory.mkdir(parents=True, exist_ok=True)
    run(
        [
            "vfox",
            "exec",
            "nodejs@24.20.0",
            "--",
            "pnpm",
            "exec",
            "tsp",
            "compile",
            str(entry.relative_to(API_ROOT)),
            "--config",
            "tspconfig.yaml",
            "--output-dir",
            str(output_directory),
        ],
        API_ROOT,
    )
    generated_file = output_directory / OPENAPI_OUTPUT
    if not generated_file.is_file():
        raise ContractError(f"TypeSpec did not emit {generated_file}")
    try:
        document = yaml.safe_load(generated_file.read_text(encoding="utf-8"))
    except (OSError, yaml.YAMLError) as error:
        raise ContractError(f"cannot read TypeSpec output {generated_file}: {error}") from error
    if not isinstance(document, dict):
        raise ContractError(f"TypeSpec output {generated_file} must be a YAML mapping")
    return document


def run(command: list[str], cwd: Path) -> None:
    try:
        completed = subprocess.run(command, cwd=cwd, check=False)
    except OSError as error:
        raise ContractError(f"cannot run {' '.join(command)}: {error}") from error
    if completed.returncode != 0:
        raise ContractError(f"command failed with exit code {completed.returncode}: {' '.join(command)}")


def normalize_common(raw: dict[str, Any], domains: Any) -> dict[str, Any]:
    document = normalize_document(raw, title="Meridian API common models")
    components = document.setdefault("components", {})
    schemas = components.setdefault("schemas", {})

    parameter_aliases: dict[str, dict[str, Any]] = {}
    for domain in domains:
        for key, parameter in domain.get("components", {}).get("parameters", {}).items():
            alias = parameter_name(key)
            schema = parameter.get("schema")
            if alias and isinstance(schema, dict):
                parameter_aliases.setdefault(alias, deepcopy(schema))

    for key, schema in list(schemas.items()):
        if not key.startswith("Parameters."):
            continue
        alias = key.removeprefix("Parameters.")
        properties = schema.get("properties", {}) if isinstance(schema, dict) else {}
        if alias not in parameter_aliases and isinstance(properties, dict) and len(properties) == 1:
            parameter_aliases[alias] = deepcopy(next(iter(properties.values())))
        del schemas[key]

    for domain in domains:
        for key, schema in domain.get("components", {}).get("schemas", {}).items():
            if not key.startswith("Parameters."):
                schemas.setdefault(key, deepcopy(schema))

    for alias, schema in parameter_aliases.items():
        schemas.setdefault(alias, schema)
    rewrite_local_refs(document, parameter_prefix="Parameters.")
    components["securitySchemes"] = security_schemes()
    document["paths"] = {}
    return document


def normalize_domain(name: str, raw: dict[str, Any], common: dict[str, Any]) -> dict[str, Any]:
    document = normalize_document(raw, title=f"Meridian {name.title()} API")
    common_schemas = set(common.get("components", {}).get("schemas", {}))
    components = document.setdefault("components", {})
    schemas = components.setdefault("schemas", {})

    renamed_parameters: dict[str, Any] = {}
    for key, parameter in components.get("parameters", {}).items():
        renamed_parameters[parameter_name(key)] = parameter
    components["parameters"] = renamed_parameters

    for key in list(schemas):
        if key in common_schemas or key.startswith("Parameters."):
            del schemas[key]
    rewrite_external_schema_refs(document, common_schemas)
    rewrite_local_refs(document, parameter_prefix="Parameters.")
    document["security"] = deepcopy(DEFAULT_SECURITY)
    components["securitySchemes"] = security_schemes()
    return document


def normalize_canonical(raw: dict[str, Any]) -> dict[str, Any]:
    document = normalize_document(raw, title="Meridian API")
    components = document.setdefault("components", {})
    schemas = components.setdefault("schemas", {})
    components["parameters"] = {
        parameter_name(key): parameter for key, parameter in components.get("parameters", {}).items()
    }
    for key in list(schemas):
        if key.startswith("Parameters."):
            del schemas[key]
    rewrite_local_refs(document, parameter_prefix="Parameters.")
    components["securitySchemes"] = security_schemes()
    document["security"] = deepcopy(DEFAULT_SECURITY)
    return document


def normalize_document(document: dict[str, Any], title: str) -> dict[str, Any]:
    result = deepcopy(document)
    info = result.setdefault("info", {})
    info["title"] = title
    info["version"] = "1.0.0"
    if title == "Meridian API":
        info["description"] = (
            "Authoritative HTTP and DTO contract. Markdown documents are explanatory only. "
            "All tenant resource operations return 404 for both absence and authorization denial."
        )
    result["servers"] = [{"url": "/"}]
    rewrite_security(result)
    annotate_generated_schema_descriptions(result)
    normalize_multipart(result)
    return result


def annotate_generated_schema_descriptions(document: dict[str, Any]) -> None:
    schemas = document.get("components", {}).get("schemas", {})
    if not isinstance(schemas, dict):
        return
    for name, schema in schemas.items():
        if not isinstance(schema, dict):
            continue
        if name == "UploadRequest":
            schema.setdefault("description", "defines the multipart upload payload for an asset source.")
        for branch in schema.get("oneOf", []):
            if not isinstance(branch, dict):
                continue
            properties = branch.get("properties", {})
            if not isinstance(properties, dict):
                continue
            for property_name, property_schema in properties.items():
                if isinstance(property_schema, dict):
                    property_schema.setdefault("description", f"specifies the {property_name} value.")


def normalize_multipart(document: dict[str, Any]) -> None:
    schemas = document.get("components", {}).get("schemas", {})
    upload = schemas.get("UploadRequest") if isinstance(schemas, dict) else None
    if isinstance(upload, dict):
        normalize_upload_file(upload)
    for path_item in document.get("paths", {}).values():
        if not isinstance(path_item, dict):
            continue
        for operation in path_item.values():
            if not isinstance(operation, dict):
                continue
            request_body = operation.get("requestBody")
            if not isinstance(request_body, dict):
                continue
            multipart = request_body.get("content", {}).get("multipart/form-data")
            if not isinstance(multipart, dict):
                continue
            schema = multipart.get("schema", {})
            if isinstance(schema, dict) and isinstance(schema.get("$ref"), str):
                schema_name = schema["$ref"].removeprefix("#/components/schemas/")
                schema = schemas.get(schema_name, schema)
            properties = schema.get("properties", {}) if isinstance(schema, dict) else {}
            file_schema = properties.get("file")
            if isinstance(schema, dict):
                normalize_upload_file(schema)


def normalize_upload_file(schema: dict[str, Any]) -> None:
    properties = schema.get("properties", {})
    file_schema = properties.get("file") if isinstance(properties, dict) else None
    if isinstance(file_schema, dict) and file_schema.get("type") in (None, "object"):
        description = file_schema.get("description")
        properties["file"] = {"type": "string", "format": "binary"}
        if description:
            properties["file"]["description"] = description


def rewrite_security(value: Any) -> None:
    if isinstance(value, dict):
        if "security" in value and isinstance(value["security"], list):
            security = value["security"]
            if any("undefined" in item for item in security if isinstance(item, dict)):
                value["security"] = []
            else:
                value["security"] = [
                    {SECURITY_NAMES.get(name, name): requirement for name, requirement in item.items()}
                    for item in security
                    if isinstance(item, dict)
                ]
        for child in value.values():
            rewrite_security(child)
    elif isinstance(value, list):
        for child in value:
            rewrite_security(child)


def rewrite_local_refs(value: Any, parameter_prefix: str) -> None:
    if isinstance(value, dict):
        reference = value.get("$ref")
        if isinstance(reference, str):
            value["$ref"] = reference.replace(
                "#/components/parameters/" + parameter_prefix,
                "#/components/parameters/",
            ).replace("#/components/schemas/" + parameter_prefix, "#/components/schemas/")
        for child in value.values():
            rewrite_local_refs(child, parameter_prefix)
    elif isinstance(value, list):
        for child in value:
            rewrite_local_refs(child, parameter_prefix)


def rewrite_external_schema_refs(value: Any, common_schemas: set[str]) -> None:
    if isinstance(value, dict):
        reference = value.get("$ref")
        if isinstance(reference, str) and reference.startswith("#/components/schemas/"):
            name = reference.removeprefix("#/components/schemas/")
            if name in common_schemas:
                value["$ref"] = f"../common/openapi.yaml#/components/schemas/{name}"
        for child in value.values():
            rewrite_external_schema_refs(child, common_schemas)
    elif isinstance(value, list):
        for child in value:
            rewrite_external_schema_refs(child, common_schemas)


def parameter_name(key: str) -> str:
    return key.removeprefix("Parameters.")


def security_schemes() -> dict[str, dict[str, str]]:
    return {
        "cookieSession": {
            "type": "apiKey",
            "in": "cookie",
            "name": "meridian_session",
            "description": "Unsafe browser requests additionally require X-CSRF-Token.",
        },
        "patBearer": {
            "type": "http",
            "scheme": "bearer",
            "bearerFormat": "pat_",
        },
    }


def validate_documents(
    canonical: dict[str, Any], domains: dict[str, dict[str, Any]], common: dict[str, Any]
) -> None:
    paths = canonical.get("paths")
    if not isinstance(paths, dict) or not paths:
        raise ContractError("main.tsp did not produce any paths")
    operation_ids: dict[str, str] = {}
    domain_paths: set[str] = set()
    for domain, document in domains.items():
        current = document.get("paths")
        if not isinstance(current, dict) or not current:
            raise ContractError(f"domains/{domain}.tsp did not produce any paths")
        for path, item in current.items():
            if path in domain_paths:
                raise ContractError(f"path {path!r} is emitted by more than one domain")
            domain_paths.add(path)
            for method, operation in item.items():
                if method not in METHODS or not isinstance(operation, dict):
                    continue
                operation_id = operation.get("operationId")
                if not isinstance(operation_id, str) or not operation_id:
                    raise ContractError(f"{domain} {method.upper()} {path} has no operationId")
                location = f"{domain} {method.upper()} {path}"
                previous = operation_ids.setdefault(operation_id, location)
                if previous != location:
                    raise ContractError(f"operationId {operation_id!r} is duplicated at {previous}")
    if domain_paths != set(paths):
        missing = sorted(domain_paths - set(paths))
        extra = sorted(set(paths) - domain_paths)
        raise ContractError(f"domain paths do not match main.tsp (missing={missing}, extra={extra})")
    canonical_ids = {
        operation.get("operationId")
        for item in paths.values()
        if isinstance(item, dict)
        for method, operation in item.items()
        if method in METHODS and isinstance(operation, dict) and operation.get("operationId")
    }
    if canonical_ids != set(operation_ids):
        raise ContractError("main.tsp operation IDs do not match the domain entry points")
    if not common.get("components", {}).get("schemas"):
        raise ContractError("common/models.tsp did not produce shared schemas")


def dump_yaml(document: dict[str, Any]) -> str:
    return yaml.safe_dump(document, sort_keys=False, default_flow_style=False, allow_unicode=False, width=120)


def write_outputs(generated: dict[Path, str]) -> None:
    for filename, content in generated.items():
        filename.parent.mkdir(parents=True, exist_ok=True)
        filename.write_text(content, encoding="utf-8")
        print(f"generated {filename.relative_to(REPOSITORY_ROOT)}")


def check_outputs(generated: dict[Path, str]) -> int:
    result = 0
    for filename, expected in generated.items():
        result |= check_output(filename, expected)
    if result == 0:
        print("checked TypeSpec projections")
    return result


def check_output(filename: Path, expected: str) -> int:
    try:
        actual = filename.read_text(encoding="utf-8")
    except OSError as error:
        print(f"contracts sync: cannot read {filename.relative_to(REPOSITORY_ROOT)}: {error}", file=sys.stderr)
        return 1
    if actual == expected:
        print(f"checked {filename.relative_to(REPOSITORY_ROOT)}")
        return 0
    diff = difflib.unified_diff(
        actual.splitlines(),
        expected.splitlines(),
        fromfile=str(filename.relative_to(REPOSITORY_ROOT)),
        tofile=f"{filename.relative_to(REPOSITORY_ROOT)} (generated)",
        lineterm="",
    )
    print("\n".join(list(diff)[:80]), file=sys.stderr)
    print(f"contracts sync: generated output {filename.relative_to(REPOSITORY_ROOT)} is stale", file=sys.stderr)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
