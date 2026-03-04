"""Simple conversation planner for dynamic follow-up prompts.

Given a previous user prompt and the assistant's response, decide whether
another prompt is needed (assistant is still asking for more info), and
generate a follow-up prompt when needed.
"""

from __future__ import annotations

import json
import re


_MORE_INFO_PATTERNS = [
    "can you provide",
    "could you provide",
    "please provide",
    "please share",
    "i need more information",
    "i need more details",
    "i need additional information",
    "what is your",
    "what are your",
    "tell me more about",
    "clarify",
    "clarification",
    "follow up question",
    "follow-up question",
]


def response_asks_for_more_info(assistant_response: str) -> bool:
    """Heuristic: return True if the assistant is clearly asking the user for more info.

    This is intentionally conservative – we only trigger when the response
    explicitly asks questions or requests more details.
    """
    if not assistant_response:
        return False
    text = assistant_response.strip()
    if not text:
        return False
    lower = text.lower()

    for pat in _MORE_INFO_PATTERNS:
        if pat in lower:
            return True

    # Generic heuristic: question directed at the user.
    if "?" in text and (" you " in lower or lower.startswith("can you") or lower.startswith("could you")):
        return True

    # Look for explicit question sentences like "What information do you have about ...?"
    if re.search(r"\bwhat else\b.*\?", lower):
        return True

    return False


def generate_followup_prompt(previous_prompt: str, assistant_response: str) -> str:
    """Generate a follow-up prompt using previous prompt + assistant response.

    The intent is to gently push the assistant to stop asking for more info
    and instead provide a complete answer, while grounding in the original task.
    """
    prev = (previous_prompt or "").strip()
    resp = (assistant_response or "").strip()

    # Keep response snippet short so prompts don't grow without bound.
    snippet = resp
    max_snippet = 400
    if len(snippet) > max_snippet:
        snippet = snippet[:max_snippet] + "..."

    base = [
        "You previously responded with:",
        "",
        snippet or "(no previous response text available)",
        "",
        "Please now answer my original request more concretely, without asking me for any more information.",
    ]
    if prev:
        base.extend(
            [
                "",
                "Original request:",
                prev,
            ]
        )
    base.extend(
        [
            "",
            "If you need to make reasonable assumptions, do so and state them briefly, but focus on providing a complete final answer.",
        ]
    )
    return "\n".join(base)


def generate_answer_for_ask_user_question(raw_sse: str) -> str:
    """Generate an answer for an askUserQuestion-style tool prompt from raw SSE.

    Heuristic:
    - Find the first assistant event with a tool item named "askUserQuestion".
    - Parse its arguments JSON to find the first question and its options.
    - If options exist, choose the first option label as our answer.
    - Build a reply that explicitly answers that question and asks the assistant
      to continue without asking more questions.
    """
    if not raw_sse:
        return (
            "Please proceed using a reasonable default answer to your question and "
            "provide the full result now without asking me any more questions."
        )

    try:
        current_data: list[str] = []
        current_event: str | None = None

        def flush_block() -> tuple[str | None, dict | None]:
            nonlocal current_data, current_event
            if not current_data:
                return None, None
            data_str = "\n".join(current_data).strip()
            current_data = []
            if not data_str or data_str == "{}":
                return None, None
            try:
                ev = json.loads(data_str)
            except json.JSONDecodeError:
                return None, None
            return current_event, ev

        # Scan SSE blocks for assistant tool items
        for raw_line in raw_sse.splitlines():
            line = raw_line.strip("\r\n")
            if line == "":
                event_type, ev = flush_block()
                current_event = None
                if not ev or event_type != "chat-in-progress":
                    continue
                if ev.get("role") != "assistant":
                    continue
                items = ev.get("items") or []
                for item in items:
                    if not isinstance(item, dict):
                        continue
                    if item.get("type") != "tool" or item.get("name") != "askUserQuestion":
                        continue
                    args_raw = item.get("arguments")
                    try:
                        args = json.loads(args_raw) if isinstance(args_raw, str) else (args_raw or {})
                    except json.JSONDecodeError:
                        args = {}
                    questions = args.get("questions") or []
                    if not questions or not isinstance(questions, list):
                        continue
                    q = questions[0]
                    question_text = q.get("question") or ""
                    options = q.get("options") or []
                    chosen_label = None
                    for opt in options:
                        if isinstance(opt, dict) and opt.get("label"):
                            chosen_label = str(opt["label"])
                            break
                    # Build answer based on what we found
                    if question_text or chosen_label:
                        parts: list[str] = []
                        if question_text and chosen_label:
                            parts.append(
                                f"For your question \"{question_text}\", please use \"{chosen_label}\" as my answer."
                            )
                        elif chosen_label:
                            parts.append(f"Please use \"{chosen_label}\" as my answer.")
                        else:
                            parts.append(
                                "Please proceed using a reasonable default answer to your question."
                            )
                        parts.append(
                            "Then provide the full result now without asking me any more questions."
                        )
                        return " ".join(parts)
                continue
            if line.startswith("event:"):
                # New event type
                event_type, _ = flush_block()
                current_event = line[6:].strip()
                continue
            if line.startswith("data:"):
                current_data.append(line[5:].strip())
                continue

        # No matching tool/question found; fall back to generic answer.
        return (
            "Please proceed using a reasonable default answer to your question and "
            "provide the full result now without asking me any more questions."
        )
    except Exception:
        return (
            "Please proceed using a reasonable default answer to your question and "
            "provide the full result now without asking me any more questions."
        )

