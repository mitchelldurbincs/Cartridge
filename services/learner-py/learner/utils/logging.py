"""Logging configuration utilities."""

from __future__ import annotations

import logging
import sys

import structlog


def configure_logging(level: str = "INFO") -> None:
    """Configure structlog and the standard logging module."""

    import os

    # Allow overriding log level via environment variable
    level = os.getenv("LOG_LEVEL", level).upper()

    # Force unbuffered output for docker containers
    sys.stdout.reconfigure(line_buffering=True)
    sys.stderr.reconfigure(line_buffering=True)

    logging.basicConfig(
        level=level,
        format="%(message)s",
        stream=sys.stdout,
        force=True,  # Override any existing logging configuration
    )

    # Ensure root logger level is set correctly
    logging.getLogger().setLevel(level)

    structlog.configure(
        wrapper_class=structlog.make_filtering_bound_logger(logging.getLevelName(level)),
        processors=[
            structlog.contextvars.merge_contextvars,
            structlog.processors.add_log_level,
            structlog.processors.TimeStamper(fmt="iso"),
            structlog.processors.JSONRenderer(),
        ],
        cache_logger_on_first_use=True,
    )

    # Log that logging has been configured
    logger = logging.getLogger(__name__)
    logger.info(f"Logging configured with level: {level}")


__all__ = ["configure_logging"]
