"""Resolve paths to eval package data and resources."""
import os

# Directory containing this file (eval/helper/)
_HELPER_DIR = os.path.dirname(os.path.abspath(__file__))
# Eval package root (eval/)
_EVAL_DIR = os.path.dirname(_HELPER_DIR)


def data_dir() -> str:
    """Return the absolute path to the eval data directory (eval/data/)."""
    return os.path.join(_EVAL_DIR, "data")


def data_path(filename: str) -> str:
    """Return the absolute path to a file under eval/data/."""
    return os.path.join(data_dir(), filename)
