"""Tests for the control plane helpers."""

from learner.control import _parse_error_detail


def test_parse_error_detail_with_json_error() -> None:
    body = '{"error": "step regression: 0 < 42"}'
    assert _parse_error_detail(body) == "step regression: 0 < 42"


def test_parse_error_detail_with_alternate_key() -> None:
    body = '{"detail": "checkpoint regression: 1 < 3"}'
    assert _parse_error_detail(body) == "checkpoint regression: 1 < 3"


def test_parse_error_detail_plain_text() -> None:
    body = "some plain text error"
    assert _parse_error_detail(body) == "some plain text error"


def test_parse_error_detail_empty_payload() -> None:
    assert _parse_error_detail("") is None
