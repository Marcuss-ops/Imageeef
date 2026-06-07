from __future__ import annotations
import logging
import re
from typing import Any
from playwright.async_api import Page

logger = logging.getLogger("image_endpoint.worker")

async def _find_editable(page: Page, selector: str | None) -> Any | None:
    if selector:
        locator = page.locator(selector)
        for idx in range(await locator.count()):
            candidate = locator.nth(idx)
            try:
                if await candidate.is_visible():
                    return candidate
            except Exception:
                continue

    candidates = [
        "[role='textbox']",
        "[contenteditable='true']",
        "input[type='text']",
        "input:not([type])",
        "textarea",
    ]
    for candidate in candidates:
        locator = page.locator(candidate)
        try:
            for idx in range(await locator.count()):
                item = locator.nth(idx)
                if await item.is_visible():
                    return item
        except Exception:
            continue
    return None

async def _click_submit(page: Page, selector: str | None) -> bool:
    if selector:
        locator = page.locator(selector)
        if await locator.count():
            await locator.first.click()
            return True

    button_names = ["Generate", "Create", "Run", "Submit", "Send"]
    for name in button_names:
        locator = page.get_by_role("button", name=re.compile(name, re.I))
        try:
            if await locator.count():
                await locator.first.click()
                return True
        except Exception:
            continue

    return False

async def _click_by_text(page: Page, text: str) -> bool:
    candidates = [
        page.get_by_role("button", name=re.compile(re.escape(text), re.I)),
        page.get_by_text(re.compile(rf"^{re.escape(text)}$", re.I)),
    ]
    for locator in candidates:
        try:
            if await locator.count():
                await locator.first.click()
                return True
        except Exception:
            continue
    return False

async def _select_4_images_layout(page: Page) -> bool:
    """
    Specifically for Flow: 
    1. Look for the 'Nano Banana' pill button at the bottom (prompt area)
    2. Click it to open the settings popup
    3. Look for the 'x4' button specifically and click it
    """
    try:
        # Step 1: Click the agent/settings pill button
        # We target buttons specifically in the bottom prompt container if possible,
        # or filter by the unique structure of that pill.
        # The pill usually has the agent name and layout info.
        
        # We search for buttons that are NOT inside the media gallery if possible
        # but a safer way is to look for the one with 'x2' or 'x1' or 'x4' in the same text
        agent_btn = page.locator("button").filter(has_text=re.compile(r"Nano Banana", re.I))
        
        # If multiple found, the one in the bottom bar is usually later in the DOM 
        # or has specific classes. Let's try to be specific about the button role.
        count = await agent_btn.count()
        if count > 0:
            # We take the LAST one because images generated with that agent 
            # might have the text, but the control pill is usually at the bottom.
            target = agent_btn.last 
            
            logger.info("clicking layout settings pill (found %d candidates)", count)
            await target.click()
            await page.wait_for_timeout(1500)
            
            # Step 2: Click the 'x4' button in the popup
            layout_x4 = page.locator("button").filter(has_text=re.compile(r"^x4$", re.I))
            
            if await layout_x4.count() > 0:
                for i in range(await layout_x4.count()):
                    btn = layout_x4.nth(i)
                    if await btn.is_visible():
                        logger.info("selecting x4 layout")
                        await btn.click()
                        await page.wait_for_timeout(1000)
                        return True
            else:
                layout_4 = page.locator("button").filter(has_text=re.compile(r"^4$", re.I))
                if await layout_4.count() > 0:
                     await layout_4.first.click()
                     return True
                     
    except Exception as e:
        logger.warning("layout selection failed: %s", e)
    return False
