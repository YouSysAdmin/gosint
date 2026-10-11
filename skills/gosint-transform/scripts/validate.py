#!/usr/bin/env python3
"""Checks gosint transform files as the server reads them, and runs a
script transform as the server runs it. Needs PyYAML (pip install pyyaml).

  validate.py PATH...                      check files or directories
  validate.py --type NAME PATH...          also accept a custom entity type
  validate.py --run FILE.yaml --input JSON [--param NAME=VALUE]...

--input is the entity: {"type": "domain", "label": "example.com",
"properties": {"domain": "example.com"}}. A vaultSecret param not given
with --param is read from the environment variable of its name, the way
the server falls back to the secret named like the param.
"""
import argparse
import json
import os
import re
import subprocess
import sys
import urllib.parse
from pathlib import Path

try:
    import yaml
except ImportError:
    sys.exit("validate.py needs PyYAML: pip install pyyaml")

# BEGIN CATALOG (written by scripts/gen, do not edit)
TYPES = json.loads(r'''{"affiliation":{"primary":"organization","fields":{"associated_individuals":"list<string>","confidence":"float","department":"string","description":"string","end_date":"string","hierarchy_level":"string","industry":"string","is_current":"bool","location":"string","organization":"string","organization_type":"string","role":"string","source":"string","start_date":"string"}},"alias":{"primary":"alias","fields":{"alias":"string","associated_entities":"list<string>","confidence":"float","context":"string","description":"string","first_seen":"string","is_active":"bool","language":"string","last_seen":"string","region":"string","source":"string","type":"string"}},"asn":{"primary":"asn_str","fields":{"asn_str":"string","cidrs":"list<ref:cidr>","country":"string","description":"string","name":"string","number":"int"}},"bankaccount":{"primary":"account_number","fields":{"account_holder":"string","account_number":"string","account_type":"string","associated_individuals":"list<string>","balance":"float","bank_name":"string","breach_source":"string","closed_date":"string","country":"string","currency":"string","iban":"string","is_compromised":"bool","is_joint":"bool","opened_date":"string","routing_number":"string","source":"string","status":"string","swift_code":"string"}},"breach":{"primary":"name","fields":{"addeddate":"string","breach":"object","breachdate":"string","dataclasses":"list<string>","description":"string","domain":"string","isfabricated":"bool","ismalware":"bool","isretired":"bool","issensitive":"bool","isspamlist":"bool","isstealerlog":"bool","issubscriptionfree":"bool","isverified":"bool","logopath":"string","modifieddate":"string","name":"string","pwncount":"int","title":"string"}},"cidr":{"primary":"network","fields":{"network":"string"}},"credential":{"primary":"username","fields":{"description":"string","fingerprint":"string","first_seen":"datetime","found_in":"string","hash_type":"string","kind":"string","last_seen":"datetime","password":"string","password_hash":"string","prefix":"string","service":"string","status":"string","url":"url","username":"string","value":"string"}},"creditcard":{"primary":"card_number","fields":{"associated_accounts":"list<string>","available_credit":"float","billing_address":"string","breach_source":"string","card_number":"string","card_type":"string","cardholder_name":"string","credit_limit":"float","cvv":"string","expiry_date":"string","is_business":"bool","is_compromised":"bool","is_prepaid":"bool","is_virtual":"bool","issued_date":"string","issuer":"string","last_used":"string","source":"string","status":"string"}},"cryptonft":{"primary":"token_id","fields":{"chain":"string","collection_name":"string","contract_address":"string","creator_address":"string","description":"string","image_url":"url","last_transfer_date":"string","metadata_url":"url","name":"string","node_id":"string","owner_address":"string","token_id":"string","wallet":"ref:cryptowallet"}},"cryptowallet":{"primary":"address","fields":{"address":"string","currency":"string","node_id":"string"}},"cryptowallettransaction":{"primary":"hash","fields":{"amount":"float","amount_usd":"float","block_hash":"string","block_number":"int","confirmations":"int","contract_address":"string","cumulative_gas_used":"int","currency":"string","date":"string","error_message":"string","function_name":"string","gas":"int","gas_price":"int","gas_used":"int","hash":"string","hop":"int","input":"string","is_error":"bool","method_id":"string","nonce":"int","source":"ref:cryptowallet","target":"ref:cryptowallet","timestamp":"string","transaction_index":"int","txreceipt_status":"string","value":"float"}},"cve":{"primary":"id","fields":{"cvss":"float","cvss_vector":"string","cwe":"string","description":"string","epss":"float","epss_percentile":"float","id":"string","kev_added":"datetime","known_exploited":"bool","published":"datetime","references":"list<string>","severity":"string","url":"url"}},"device":{"primary":"device_id","fields":{"associated_users":"list<string>","browser":"string","browser_version":"string","device_id":"string","first_seen":"string","ip_addresses":"list<string>","is_desktop":"bool","is_mobile":"bool","is_tablet":"bool","last_seen":"string","location":"string","mac_address":"string","manufacturer":"string","model":"string","os":"string","os_version":"string","screen_resolution":"string","source":"string","type":"string","user_agent":"string"}},"dnsrecord":{"primary":"value","fields":{"associated_domains":"list<string>","description":"string","first_seen":"string","is_active":"bool","is_suspicious":"bool","last_seen":"string","malware_family":"string","nameserver":"string","priority":"int","record_type":"string","source":"string","threat_level":"string","ttl":"int","value":"string"}},"document":{"primary":"title","fields":{"author":"string","created_date":"string","description":"string","doc_type":"string","file_size":"int","hash_md5":"string","hash_sha1":"string","hash_sha256":"string","is_encrypted":"bool","is_password_protected":"bool","keywords":"list<string>","language":"string","location":"string","modified_date":"string","page_count":"int","source":"string","source_url":"string","subject":"string","title":"string"}},"domain":{"primary":"domain","fields":{"alive":"bool","check_error":"string","checked_at":"datetime","domain":"string","http_status":"int","root":"bool"}},"email":{"primary":"email","fields":{"email":"email"}},"event":{"primary":"title","fields":{"description":"string","ends_at":"datetime","location":"ref:location","participants":"list<string>","source":"url","starts_at":"datetime","title":"string"}},"file":{"primary":"filename","fields":{"accessed_date":"string","created_date":"string","description":"string","file_size":"int","file_type":"string","filename":"string","hash_md5":"string","hash_sha1":"string","hash_sha256":"string","is_archive":"bool","is_audio":"bool","is_executable":"bool","is_image":"bool","is_malicious":"bool","is_text":"bool","is_video":"bool","malware_family":"string","mime_type":"string","modified_date":"string","path":"string","source":"string","threat_level":"string"}},"finding":{"primary":"title","fields":{"description":"string","evidence":"string","last_seen":"datetime","location":"string","missing_since":"datetime","observed_at":"datetime","references":"list<string>","rule":"string","severity":"string","source":"string","status":"string","tags":"list<string>","title":"string"}},"gitrepo":{"primary":"url","fields":{"credential_key":"string","default_branch":"string","description":"string","host":"string","name":"string","owner":"string","stars":"int","url":"url","visibility":"string"}},"gravatar":{"primary":"hash","fields":{"about_me":"string","default_image":"string","display_name":"string","exists":"bool","force_default":"bool","hash":"string","large_url":"url","location":"string","profile_url":"url","rating":"string","size":"int","src":"url","thumbnail_url":"url"}},"host":{"primary":"hostname","fields":{"hostname":"string","ip":"ref:ip","notes":"string","os":"string","provider":"string","role":"string"}},"individual":{"primary":"full_name","fields":{"age":"int","aliases":"list<string>","annual_income":"float","arrest_history":"list<string>","assets":"list<string>","bank_accounts":"list<string>","birth_date":"string","birth_place":"ref:location","blood_type":"string","cause_of_death":"string","children_count":"int","children_names":"list<string>","citizenship":"list<string>","civil_cases":"list<string>","confidence":"float","convictions":"list<string>","credit_score":"int","criminal_record":"bool","current_address":"ref:location","death_date":"string","death_place":"ref:location","degrees":"list<string>","device_ids":"list<string>","disabilities":"list<string>","distinguishing_marks":"list<string>","dna_profile":"string","drivers_license":"string","education_level":"string","email_addresses":"list<ref:email>","emergency_contact":"string","employer":"string","employer_address":"ref:location","employment_history":"list<string>","ethnicity":"string","eye_color":"string","facial_recognition_id":"string","fingerprints":"string","first_name":"string","first_seen":"string","full_name":"string","gender":"string","hair_color":"string","height":"string","hobbies":"list<string>","ip_addresses":"list<ref:ip>","iris_scan":"string","is_deceased":"bool","is_smoker":"bool","job_title":"string","languages_spoken":"list<string>","last_name":"string","last_seen":"string","last_updated":"string","maiden_name":"string","marital_status":"string","middle_name":"string","national_id":"string","nationality":"string","net_worth":"float","notes":"string","occupation":"string","parents_names":"list<string>","passport_number":"string","personality_traits":"list<string>","phone_numbers":"list<ref:phone>","place_of_residence":"string","political_affiliation":"string","previous_addresses":"list<ref:location>","religion":"string","schools_attended":"list<string>","security_clearance":"string","skin_tone":"string","social_media_profiles":"list<string>","social_security_number":"string","source":"string","spouse_name":"string","src":"string","tax_id":"string","usernames":"list<string>","voice_print":"string","voter_id":"string","weight":"string"}},"ip":{"primary":"address","fields":{"address":"string","cdn":"string","city":"string","country":"string","isp":"string","latitude":"float","longitude":"float","scope":"string"}},"leak":{"primary":"name","fields":{"leak":"list<object>","name":"string"}},"location":{"primary":"address","fields":{"address":"string","city":"string","country":"string","latitude":"float","longitude":"float","zip":"string"}},"malware":{"primary":"name","fields":{"analysis_report":"string","c2_servers":"list<string>","capabilities":"list<string>","description":"string","distribution_methods":"list<string>","encryption_methods":"list<string>","evasion_techniques":"list<string>","family":"string","first_seen":"string","indicators":"list<string>","is_active":"bool","last_seen":"string","name":"string","persistence_mechanisms":"list<string>","sample_hashes":"list<string>","source":"string","targets":"list<string>","threat_level":"string","type":"string"}},"message":{"primary":"message_id","fields":{"attachments":"list<string>","content":"string","entities":"list<string>","is_read":"bool","is_reply":"bool","is_suspicious":"bool","keywords":"list<string>","language":"string","message_id":"string","message_type":"string","parent_message_id":"string","platform":"string","recipient":"string","sender":"string","sentiment":"string","source":"string","subject":"string","threat_level":"string","timestamp":"string"}},"organization":{"primary":"name","fields":{"address":"ref:location","aliases":"list<string>","country":"string","description":"string","email":"email","employees":"int","executives":"list<ref:individual>","founded":"string","industry":"string","legal_name":"string","name":"string","parent":"string","phone":"string","registration_country":"string","registration_number":"string","status":"string","type":"string","website":"url"}},"phone":{"primary":"number","fields":{"carrier":"string","country":"string","number":"string"}},"photo":{"primary":"title","fields":{"camera":"string","description":"string","hash":"string","latitude":"float","longitude":"float","taken_at":"datetime","title":"string","url":"url"}},"phrase":{"primary":"text","fields":{"text":"any"}},"port":{"primary":"number","fields":{"banner":"string","number":"int","protocol":"string","service":"string","state":"string"}},"reputationscore":{"primary":"entity_id","fields":{"category":"string","confidence":"float","description":"string","entity_id":"string","entity_type":"string","factors":"list<string>","historical_scores":"list<object>","last_updated":"string","provider":"string","recommendations":"list<string>","risk_level":"string","score":"float","score_type":"string","source":"string","trend":"string"}},"riskprofile":{"primary":"entity_id","fields":{"assessment_date":"string","assessor":"string","attack_vectors":"list<string>","compliance_risks":"list<string>","confidence":"float","data_classification":"string","entity_id":"string","entity_type":"string","exposure_surface":"string","financial_impact":"string","last_updated":"string","mitigation_strategies":"list<string>","monitoring_recommendations":"list<string>","next_review_date":"string","operational_impact":"string","overall_risk_score":"float","reputation_impact":"string","risk_factors":"list<string>","risk_level":"string","source":"string","threat_actors":"list<string>","vulnerabilities":"list<string>"}},"script":{"primary":"script_id","fields":{"author":"string","content":"string","created_date":"string","dependencies":"list<string>","description":"string","file_path":"string","file_size":"int","functions":"list<string>","hash_md5":"string","hash_sha1":"string","hash_sha256":"string","is_malicious":"bool","language":"string","malware_family":"string","minified":"bool","modified_date":"string","name":"string","obfuscated":"bool","script_id":"string","source":"string","threat_level":"string","type":"string","url":"string","version":"string"}},"session":{"primary":"session_id","fields":{"activities":"list<string>","browser":"string","device_type":"string","duration":"int","end_time":"string","ip_address":"string","is_active":"bool","is_suspicious":"bool","location":"string","os":"string","service":"string","session_id":"string","source":"string","start_time":"string","user_agent":"string","user_id":"string"}},"socialaccount":{"primary":"id","fields":{"associated_emails":"list<string>","associated_phones":"list<string>","bio":"string","created_at":"string","display_name":"string","followers_count":"int","following_count":"int","id":"string","is_private":"bool","is_suspended":"bool","location":"string","platform":"string","posts_count":"int","profile_picture_url":"string","profile_url":"string","username":"ref:username","verified":"bool"}},"sslcertificate":{"primary":"subject","fields":{"crl_status":"string","fingerprint_sha1":"string","fingerprint_sha256":"string","first_seen":"string","is_expired":"bool","is_self_signed":"bool","is_valid":"bool","is_wildcard":"bool","issuer":"string","key_size":"int","key_type":"string","last_seen":"string","ocsp_status":"string","san_domains":"list<string>","serial_number":"string","signature_algorithm":"string","source":"string","subject":"string","valid_from":"string","valid_until":"string"}},"technology":{"primary":"name","fields":{"category":"string","name":"string","source":"string","version":"string"}},"username":{"primary":"value","fields":{"last_seen":"string","platform":"string","value":"string"}},"vehicle":{"primary":"plate","fields":{"color":"string","country":"string","make":"string","model":"string","plate":"string","vin":"string","year":"int"}},"vulnerability":{"primary":"name","fields":{"affected":"string","cve":"string","cvss":"float","cvss_vector":"string","cwe":"string","description":"string","discovered_at":"datetime","fixed_at":"datetime","name":"string","references":"list<string>","remediation":"string","severity":"string","status":"string"}},"weapon":{"primary":"name","fields":{"accessories":"list<string>","action_type":"string","barrel_length":"string","caliber":"string","capacity":"string","condition":"string","country_of_origin":"string","date_found":"string","description":"string","evidence_number":"string","firing_mode":"string","legal_status":"string","location_found":"string","manufacturer":"string","model":"string","modifications":"list<string>","name":"string","notes":"string","overall_length":"string","owner":"string","price":"float","purchase_date":"string","purchase_location":"string","registration_status":"string","safety_features":"list<string>","serial_number":"string","source":"string","type":"string","weight":"string","year_manufactured":"int"}},"website":{"primary":"url","fields":{"active":"bool","content":"string","description":"string","domain":"ref:domain","final_url":"url","headers":"object","redirects":"list<url>","status_code":"int","technologies":"list<string>","title":"string","url":"url"}},"webtracker":{"primary":"tracker_id","fields":{"company":"string","cookie_duration":"int","data_collected":"list<string>","domain":"string","first_seen":"string","gdpr_compliant":"bool","is_active":"bool","is_third_party":"bool","last_seen":"string","name":"string","opt_out_url":"string","privacy_policy":"string","purpose":"string","risk_level":"string","script_url":"string","source":"string","tracker_id":"string","type":"string"}},"whois":{"primary":"domain","fields":{"city":"string","country":"string","creation_date":"string","domain":"ref:domain","email":"ref:email","expiration_date":"string","organization":"ref:organization","registrar":"string","registry_domain_id":"string"}}}''')
BUILTIN = json.loads(r'''["asn_to_cidrs","cidr_to_ips","cryptowallet_to_nfts","cryptowallet_to_transactions","cve_to_details","cve_to_vulnerability","domain_alive","domain_root_only","domain_to_asn","domain_to_dehashed","domain_to_dns","domain_to_history","domain_to_hunter","domain_to_ip","domain_to_root_domain","domain_to_securitytrails","domain_to_subdomains","domain_to_tls","domain_to_urlscan","domain_to_virustotal","domain_to_website","domain_to_whois","domain_to_whois_history","email_to_breaches","email_to_dehashed","email_to_domain","email_to_domains","email_to_gravatar","email_to_infostealers","email_to_username","fields_to_entities","files_to_entities","finding_to_vulnerability","individual_to_dehashed","individual_to_domains","ip_drop_cdn","ip_public_only","ip_to_abuseipdb","ip_to_asn","ip_to_censys","ip_to_dehashed","ip_to_domain","ip_to_fraudscore","ip_to_greynoise","ip_to_infos","ip_to_ports","ip_to_shodan","ip_to_virustotal","location_to_coordinates","n8n_connector","org_to_asn","org_to_domains","phone_to_carrier","phone_to_dehashed","phone_to_infostealers","tech_detect","username_to_accounts","username_to_dehashed","username_to_infostealers","vulnerability_to_details","website_to_crawler","website_to_domain","website_to_links","website_to_subdomains","website_to_text","website_to_webtrackers"]''')
# END CATALOG

NAME_RE = re.compile(r"^[a-z0-9][a-z0-9_-]{0,127}$")
PARAM_RE = re.compile(r"^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$")
RELATION_RE = re.compile(r"^[A-Z][A-Z0-9_]{0,63}$")
HEADER_RE = re.compile(r"^[A-Za-z0-9-]+$")
PLACEHOLDER_RE = re.compile(r"\{\{\s*([a-zA-Z_][a-zA-Z0-9_.]*)\s*\}\}")
PARAM_TYPES = ("string", "url", "number", "select", "bool", "vaultSecret")
PASSED_ENV = ("PATH", "HOME", "USER", "SHELL", "LANG", "LANGUAGE", "TMPDIR", "TMP", "TEMP", "TZ",
              "SYSTEMROOT", "WINDIR", "COMSPEC", "PATHEXT", "USERPROFILE", "APPDATA", "LOCALAPPDATA")
MAX_LINE = 1 << 20

# The server decodes strictly: these are the only keys, with their kinds.
OPTION = {"label": "str", "value": "str"}
PARAM = {"name": "str", "type": "str", "description": "str", "required": "bool", "default": "str",
         "options": ("list", OPTION)}
MANIFEST = {"name": "str", "description": "str", "category": "str", "icon": "str", "input": "str",
            "output": "str", "run": "str", "timeout": "num", "params": ("list", PARAM)}
TEMPLATE = {
    "name": "str", "description": "str", "category": "str", "version": "num",
    "input": ("obj", {"type": "str", "key": "str"}),
    "request": ("obj", {"method": "str", "url": "str", "headers": "strmap", "params": "strmap",
                        "body": "str", "timeout": "num"}),
    "response": ("obj", {"expect": "str", "map": "strmap"}),
    "output": ("obj", {"type": "str", "is_array": "bool", "array_path": "str", "relation": "str"}),
    "secrets": ("list", {"name": "str", "required": "bool", "description": "str"}),
    "retry": ("obj", {"max_retries": "int", "backoff_factor": "num", "retry_on_status": ("list", "int")}),
}


class Loader(yaml.SafeLoader):
    """SafeLoader refusing a key given twice, as the server does."""


def _mapping(loader, node, deep=False):
    seen = set()
    for key_node, _ in node.value:
        key = loader.construct_object(key_node, deep=deep)
        if key in seen:
            raise yaml.constructor.ConstructorError(None, None, f"mapping key {key!r} already defined",
                                                    key_node.start_mark)
        seen.add(key)
    return loader.construct_mapping(node, deep=True)


Loader.add_constructor(yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG, _mapping)


def yaml_kind(v) -> str:
    if isinstance(v, dict):
        return "a mapping"
    if isinstance(v, list):
        return "a list"
    if isinstance(v, str):
        return "a string"
    return type(v).__name__


def decode(v, spec, path: str, errs: list):
    """v read into spec the way yaml.v3 fills a Go struct, errors named by path."""
    where = path or "the file"
    if isinstance(spec, dict):
        spec = ("obj", spec)
    if spec == "str":
        if v is None:
            return ""
        if isinstance(v, bool):
            return "true" if v else "false"
        if isinstance(v, (str, int, float)):
            return str(v)
        errs.append(f"{where}: {yaml_kind(v)} where a string goes")
        return ""
    if spec in ("num", "int"):
        if v is None:
            return 0
        if isinstance(v, bool) or not isinstance(v, (int, float)) or (spec == "int" and isinstance(v, float)):
            errs.append(f"{where}: {yaml_kind(v)} where {'an integer' if spec == 'int' else 'a number'} goes"
                        + (" (drop the quotes)" if isinstance(v, str) else ""))
            return 0
        return v
    if spec == "bool":
        if v is None:
            return False
        if not isinstance(v, bool):
            errs.append(f"{where}: {yaml_kind(v)} where true or false goes")
            return False
        return v
    if spec == "strmap":
        if v is None:
            return {}
        if not isinstance(v, dict):
            errs.append(f"{where}: {yaml_kind(v)} where a mapping goes")
            return {}
        return {str(k): decode(x, "str", f"{path}.{k}", errs) for k, x in v.items()}
    kind, inner = spec
    if kind == "list":
        if v is None:
            return []
        if not isinstance(v, list):
            errs.append(f"{where}: {yaml_kind(v)} where a list goes")
            return []
        return [decode(x, inner, f"{path}[{i}]", errs) for i, x in enumerate(v)]
    # An object: every key known, absent ones left out.
    if v is None:
        return None
    if not isinstance(v, dict):
        errs.append(f"{where}: {yaml_kind(v)} where a mapping goes")
        return None
    out = {}
    for k, x in v.items():
        sub = f"{path}.{k}" if path else str(k)
        if k not in inner:
            errs.append(f"{sub}: unknown key, the server refuses the file (known: {', '.join(inner)})")
            continue
        out[k] = decode(x, inner[k], sub, errs)
    return out


def known_type(name: str, extra: set) -> bool:
    return name in TYPES or name in extra


class Report:
    def __init__(self, file: Path):
        self.file = file
        self.errors: list[str] = []
        self.warnings: list[str] = []
        self.name = ""
        self.kind = ""
        self.summary = ""


def load(file: Path, rep: Report):
    try:
        doc = next(yaml.load_all(file.read_text(encoding="utf-8"), Loader=Loader), None)
    except (yaml.YAMLError, UnicodeDecodeError) as e:
        rep.errors.append(f"not YAML: {e}")
        return None
    if not isinstance(doc, dict):
        rep.errors.append("the file is not a YAML mapping")
        return None
    return doc


def check_manifest(doc: dict, file: Path, rep: Report, extra: set) -> dict:
    errs = rep.errors
    m = decode(doc, ("obj", MANIFEST), "", errs) or {}
    name = m.get("name", "").strip()
    inp = m.get("input", "").strip().lower()
    out = m.get("output", "").strip().lower()
    run = m.get("run", "").strip()
    timeout = m.get("timeout") or 60
    rep.name, rep.kind = name, "script"

    if not NAME_RE.match(name):
        errs.append("name must be lowercase letters, digits, dash and underscore")
    if not m.get("category", "").strip():
        errs.append("category is required")
    for t in (inp, out):
        if t and not known_type(t, extra):
            errs.append(f"type {t!r} is not a known entity type (a custom one? pass --type {t})")
    if not run:
        errs.append("run is required")
    if not 1 <= timeout <= 3600:
        errs.append("timeout must be between 1 and 3600 seconds")

    seen = set()
    for p in m.get("params", []):
        pname, ptype = p.get("name", ""), p.get("type") or "string"
        p["type"] = ptype
        if not PARAM_RE.match(pname):
            errs.append(f"param {pname!r} is not a valid name")
        elif pname in seen:
            errs.append(f"param {pname!r} is declared twice")
        elif ptype not in PARAM_TYPES:
            errs.append(f"param {pname!r} has an unknown type {ptype!r} (one of {', '.join(PARAM_TYPES)})")
        elif ptype == "select" and not p.get("options"):
            errs.append(f"param {pname!r} is a select with no options")
        seen.add(pname)
        default = p.get("default", "")
        if not default:
            continue
        if ptype == "number":
            try:
                float(default)
            except ValueError:
                rep.warnings.append(f"param {pname!r}: default {default!r} is not a number, a launch refuses it")
        elif ptype == "bool" and default not in ("true", "false"):
            rep.warnings.append(f"param {pname!r}: a bool default is \"true\" or \"false\"")
        elif ptype == "select" and default not in [o.get("value") for o in p.get("options", [])]:
            rep.warnings.append(f"param {pname!r}: default {default!r} is not one of the option values")
        elif ptype == "vaultSecret":
            rep.warnings.append(f"param {pname!r}: a vaultSecret default is never read, the secret named "
                                f"{pname!r} is the fallback")

    if run:
        check_run(run, file.parent, rep)
    rep.summary = f"{inp or 'any'} -> {out or 'mixed'}"
    m.update(name=name, input=inp, output=out, run=run, timeout=timeout)
    return m


def check_run(run: str, where: Path, rep: Report) -> None:
    if any(c in run for c in "|&;<>$`\"'~*"):
        rep.warnings.append("run is split on spaces and started without a shell: no pipes, quotes, "
                            "variables or ~ there, put them in the script")
    args = run.split()
    cmd = args[0]
    if "/" in cmd:
        path = (where / cmd) if not os.path.isabs(cmd) else Path(cmd)
        if not path.exists():
            rep.warnings.append(f"run: {cmd} is not in {where}")
        elif os.name != "nt" and not os.access(path, os.X_OK):
            rep.warnings.append(f"run: {cmd} has no exec bit (chmod +x, and a shebang line)")
    for a in args[1:]:
        if Path(a).suffix and not a.startswith("-"):
            path = where / a
            if not path.exists():
                rep.warnings.append(f"run: {a} is not in {where}")
            elif path.suffix == ".py":
                try:
                    compile(path.read_text(encoding="utf-8"), str(path), "exec")
                except SyntaxError as e:
                    rep.errors.append(f"{a}:{e.lineno}: {e.msg}")


def check_template(doc: dict, rep: Report, extra: set) -> None:
    errs = rep.errors
    t = decode(doc, ("obj", TEMPLATE), "", errs) or {}
    inp = t.get("input") or {}
    req = t.get("request") or {}
    resp = t.get("response") or {}
    out = t.get("output") or {}
    name = t.get("name", "").strip()
    rep.name, rep.kind = name, "template"

    in_type = inp.get("type", "").strip().lower()
    out_type = out.get("type", "").strip().lower()
    key = inp.get("key", "").strip() or "nodeLabel"
    method = (req.get("method", "").strip() or "GET").upper()
    expect = resp.get("expect", "").strip() or "json"
    relation = out.get("relation", "").strip() or "RELATED_TO"
    timeout = req.get("timeout") or 30
    rmap = resp.get("map", {})

    if isinstance(doc.get("input"), str):
        errs.append("input: a template takes a mapping, input: {type: ..., key: ...} (a manifest takes a name)")
    if not NAME_RE.match(name):
        errs.append("name must be lowercase letters, digits, dash and underscore")
    if not t.get("category", "").strip():
        errs.append("category is required")
    for typ in (in_type, out_type):
        if not known_type(typ, extra):
            errs.append(f"type {typ!r} is not a known entity type"
                        + (f" (a custom one? pass --type {typ})" if typ else ""))
    if method not in ("GET", "POST"):
        errs.append("request.method must be GET or POST")
    u = urllib.parse.urlsplit(req.get("url", "").replace("{{", "x"))
    if u.scheme not in ("http", "https"):
        errs.append("request.url must be an http or https URL")
    if not 1 <= timeout <= 300:
        errs.append("request.timeout must be between 1 and 300 seconds")
    for h in req.get("headers", {}):
        if not HEADER_RE.match(h):
            errs.append(f"request header {h!r} is not a valid name")
    if expect not in ("json", "xml", "text"):
        errs.append("response.expect must be json, xml or text")
    if not rmap:
        errs.append("response.map must map at least one output field")
    if not RELATION_RE.match(relation):
        errs.append("output.relation must be UPPER_SNAKE_CASE")
    secrets = [s.get("name", "") for s in t.get("secrets", [])]
    for s in secrets:
        if not PARAM_RE.match(s):
            errs.append(f"secret {s!r} is not a valid secret name")
    retry = t.get("retry")
    if retry is not None:
        mr, bf = retry.get("max_retries", 0), retry.get("backoff_factor", 0)
        if not 0 <= mr <= 10 or not 0.1 <= bf <= 10:
            errs.append("retry needs max_retries between 0 and 10 and backoff_factor between 0.1 and 10 "
                        "(a retry block sets both, an absent one is 0)")

    # What fails only when it runs.
    texts = [req.get("url", ""), req.get("body", "")] + list(req.get("headers", {}).values()) \
        + list(req.get("params", {}).values())
    allowed = {key} | {f"secrets.{s}" for s in secrets}
    for text in texts:
        for ph in PLACEHOLDER_RE.findall(text):
            if ph not in allowed:
                rep.warnings.append(f"{{{{{ph}}}}} has no value at run time, every input fails "
                                    f"(the input key is {{{{{key}}}}}, secrets are {{{{secrets.NAME}}}} declared under secrets)")
    if key not in ("nodeLabel", "label") and in_type in TYPES and key not in TYPES[in_type]["fields"]:
        rep.warnings.append(f"input.key {key!r} is not a field of {in_type}, an input lacking it fails")
    in_place = in_type == out_type and not out.get("is_array")
    if out_type in TYPES:
        fields = TYPES[out_type]["fields"]
        primary = TYPES[out_type]["primary"]
        for f in rmap:
            if f not in fields:
                rep.warnings.append(f"response.map: {f!r} is not a field of {out_type}, kept as a custom field")
        if not in_place and primary and primary not in rmap:
            rep.warnings.append(f"response.map does not fill {out_type}'s primary field {primary!r}, "
                                "the entities have no identity of their own")
    if expect == "xml" and out.get("is_array"):
        rep.warnings.append("is_array is not read with expect: xml, the answer makes one entity")
    rep.summary = f"{in_type} -> {out_type}" + (" in place" if in_place else f" {relation}")


def check_file(file: Path, extra: set) -> Report:
    rep = Report(file)
    doc = load(file, rep)
    if doc is None:
        return rep
    if str(doc.get("run") or "").strip():
        check_manifest(doc, file, rep, extra)
    else:
        check_template(doc, rep, extra)
    if rep.name in BUILTIN:
        rep.errors.append(f"{rep.name} is the name of a built-in transform, the file is left out")
    return rep


def files_of(paths: list[str]) -> list[Path]:
    out = []
    for p in map(Path, paths):
        if p.is_dir():
            for f in sorted(p.iterdir()):
                if f.is_file() and f.suffix.lower() in (".yaml", ".yml") and not f.name.startswith("."):
                    out.append(f)
            subdirs = [d.name for d in p.iterdir() if d.is_dir() and not d.name.startswith(".")]
            if subdirs:
                print(f"note: the server reads {p} flat, {', '.join(subdirs)} would be ignored there")
        else:
            out.append(p)
    return out


def validate(paths: list[str], extra: set) -> int:
    failed = 0
    names: dict[str, Path] = {}
    for f in files_of(paths):
        rep = check_file(f, extra)
        if rep.name and not rep.errors:
            if rep.name in names:
                rep.errors.append(f"{rep.name} is declared by {names[rep.name].name} too, one of them is left out")
            names.setdefault(rep.name, f)
        status = "FAIL" if rep.errors else "ok"
        print(f"{status:4} {f.name}: {rep.kind} {rep.name} {rep.summary}".rstrip())
        for e in rep.errors:
            print(f"     error: {e}")
        for w in rep.warnings:
            print(f"     warn:  {w}")
        failed += bool(rep.errors)
    return 1 if failed else 0


def scrubbed_env() -> dict:
    return {k: v for k, v in os.environ.items()
            if k.upper() in PASSED_ENV or (len(k) > 3 and k[:3].upper() == "LC_")}


def run_script(file: Path, node: dict, given: dict, extra: set) -> int:
    rep = Report(file)
    doc = load(file, rep)
    if doc is None or not str(doc.get("run") or "").strip():
        print(f"{file}: not a manifest with a run line")
        return 1
    m = check_manifest(doc, file, rep, extra)
    if rep.errors:
        for e in rep.errors:
            print(f"error: {e}")
        return 1

    declared = {p["name"]: p for p in m.get("params", [])}
    for k in given:
        if k not in declared:
            print(f"error: params.{k} is not a parameter of this transform")
            return 1
    params = {}
    for name, p in declared.items():
        v = given.get(name, "")
        if p["type"] == "vaultSecret":
            v = v or os.environ.get(name, "")
            if not v and p.get("required"):
                print(f"error: params.{name} needs a secret, pass --param {name}=... or set ${name}")
                return 1
        else:
            v = v or p.get("default", "")
            if not v and p.get("required"):
                print(f"error: params.{name} is required")
                return 1
        params[name] = v

    node = {"id": node.get("id") or "00000000-0000-7000-8000-000000000000", "type": node.get("type", ""),
            "label": node.get("label", ""), "properties": node.get("properties") or {}}
    if m["input"] and node["type"] != m["input"]:
        print(f"warn: the transform takes {m['input']}, the input is {node['type'] or 'untyped'}")
    stdin = json.dumps({"input": node, "params": params}, ensure_ascii=False)
    print(f"$ {m['run']}   (in {file.parent.resolve()}, timeout {m['timeout']:g}s, server environment only)")

    try:
        proc = subprocess.Popen(m["run"].split(), cwd=file.parent, env=scrubbed_env(), stdin=subprocess.PIPE,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    except OSError as e:
        print(f"FAIL: {e}")
        return 1
    try:
        out, err = proc.communicate(stdin.encode(), timeout=m["timeout"])
    except subprocess.TimeoutExpired:
        proc.kill()
        out, err = proc.communicate()
        print(f"FAIL: stopped after {m['timeout']:g}s, the server fails this input")
        show_lines(out, node, extra)
        return 1

    problems = show_lines(out, node, extra)
    if proc.returncode != 0:
        tail = err.decode(errors="replace")[-2000:].strip().splitlines()
        reason = tail[-1].strip()[:300] if tail else ""
        print(f"FAIL: exit {proc.returncode}" + (f": {reason}" if reason else "")
              + "   (this line is what the sketch log shows)")
        return 1
    if err.strip():
        print(f"stderr (not shown by the server on success): {err.decode(errors='replace').strip()[:500]}")
    return 1 if problems else 0


def show_lines(out: bytes, node: dict, extra: set) -> int:
    """Prints what the server makes of each stdout line, counts the refused ones."""
    problems = 0
    in_primary = TYPES.get(node["type"], {}).get("primary")
    for raw in out.splitlines():
        raw = raw.strip()
        if not raw:
            continue
        if len(raw) > MAX_LINE:
            print(f"  ! a line is over {MAX_LINE >> 10} KB, the server stops reading")
            return problems + 1
        try:
            line = json.loads(raw)
        except ValueError:
            print(f"  ! not JSON, skipped: {raw[:120].decode(errors='replace')}")
            problems += 1
            continue
        if not isinstance(line, dict):
            print(f"  ! not an object, skipped: {raw[:120].decode(errors='replace')}")
            problems += 1
        elif line.get("log"):
            level = "warn" if line.get("level") in ("warning", "warn", "error") else "info"
            print(f"  log {level}: {line['log']}")
        elif line.get("update") is not None:
            upd = line["update"]
            print(f"  update {node['type']} {node['label']}: {json.dumps(upd, ensure_ascii=False)[:300]}")
            old = node["properties"].get(in_primary) or node["label"]
            if in_primary and in_primary in upd and str(upd[in_primary]) != str(old):
                print(f"  ! update changes the primary field {in_primary!r}: that writes ANOTHER entity, "
                      "this one stays as it was")
                problems += 1
        elif line.get("type"):
            typ = str(line["type"]).lower()
            rel = (line.get("relation") or "").strip() or "RELATED_TO"
            props = line.get("properties") or {}
            arrow = f"<-{rel}-" if line.get("reverse") else f"-{rel}->"
            primary = TYPES.get(typ, {}).get("primary")
            ident = props.get(primary) if primary else None
            print(f"  {node['label']} {arrow} {typ} {ident or line.get('label') or '?'}")
            if not known_type(typ, extra):
                print(f"  ! {typ!r} is not a known entity type, refused")
                problems += 1
            if not RELATION_RE.match(rel):
                print(f"  ! relation {rel!r} is not UPPER_SNAKE_CASE, refused")
                problems += 1
            if primary and not ident and not line.get("label"):
                print(f"  ! no {primary!r} and no label, the entity is refused unless its type computes a label")
                problems += 1
            for k in props:
                if typ in TYPES and k not in TYPES[typ]["fields"]:
                    print(f"    (custom field {k!r}, kept as given)")
        else:
            print(f"  ! names no type, update or log, skipped: {raw[:120].decode(errors='replace')}")
            problems += 1
    return problems


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("paths", nargs="*", help="transform files or directories to check")
    ap.add_argument("--type", action="append", default=[], help="a custom entity type the server has")
    ap.add_argument("--run", metavar="FILE", help="run this manifest's script on --input")
    ap.add_argument("--input", help="the input entity as JSON")
    ap.add_argument("--param", action="append", default=[], metavar="NAME=VALUE")
    a = ap.parse_args()
    extra = {t.lower() for t in a.type}

    if a.run:
        if not a.input:
            ap.error("--run needs --input")
        given = dict(p.split("=", 1) for p in a.param if "=" in p)
        return run_script(Path(a.run), json.loads(a.input), given, extra)
    if not a.paths:
        ap.error("give files or directories to check, or --run")
    return validate(a.paths, extra)


if __name__ == "__main__":
    sys.exit(main())
