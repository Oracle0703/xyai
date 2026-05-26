from pathlib import Path
import re


ROOT = Path(__file__).resolve().parents[1]
PAGE = ROOT / "deploy" / "static" / "image-gen.html"


def test_static_image_gen_page_is_self_contained():
    html = PAGE.read_text(encoding="utf-8")

    assert "<title>文生图工具</title>" in html
    assert not re.search(r"<script[^>]+src=", html, re.IGNORECASE)
    assert not re.search(r"<link[^>]+rel=[\"']stylesheet", html, re.IGNORECASE)
    assert "id=\"gatewayBase\"" in html
    assert "id=\"apiKey\"" in html
    assert "id=\"prompt\"" in html


def test_static_image_gen_page_calls_gateway_and_keeps_local_history():
    html = PAGE.read_text(encoding="utf-8")

    assert "/v1/images/generations" in html
    assert "/v1/images/edits" in html
    assert "192.168." not in html
    assert "DEFAULT_GATEWAY_BASE = window.location.origin" in html
    assert "Authorization" in html
    assert "Bearer ${apiKey}" in html
    assert "response_format" in html
    assert "b64_json" in html
    assert "xyai-image-gen-history-v1" in html
    assert "localStorage" in html
    assert "MAX_HISTORY" in html


def test_static_image_gen_page_supports_image_edit_upload_and_paste():
    html = PAGE.read_text(encoding="utf-8")

    assert "id=\"modeEdit\"" in html
    assert "id=\"sourceImage\"" in html
    assert "accept=\"image/*\"" in html
    assert "addEventListener('paste'" in html
    assert "new FormData()" in html
    assert "formData.append('image'" in html
    assert "clipboardData" in html
