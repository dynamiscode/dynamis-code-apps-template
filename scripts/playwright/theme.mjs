export async function setAccountTheme(page, theme) {
  const returnTo = new URL(page.url());
  await page.goto("/account");
  const profile = page.locator('form[action="/account/profile"]');
  const previousTheme = await profile.locator('[name="theme"]').inputValue();

  const update = async (nextTheme) => {
    const currentProfile = page.locator('form[action="/account/profile"]');
    const response = await page.request.post("/account/profile", {
      maxRedirects: 0,
      form: {
        csrf: await currentProfile.locator('input[name="csrf"]').inputValue(),
        timezone: await currentProfile.locator('[name="timezone"]').inputValue(),
        locale: await currentProfile.locator('[name="locale"]').inputValue(),
        display_name: await currentProfile.locator('[name="display_name"]').inputValue(),
        theme: nextTheme,
      },
    });
    if (response.status() !== 303) throw new Error(`Account theme update returned HTTP ${response.status()}`);
  };

  await update(theme);
  await page.goto(returnTo.href);

  return async () => {
    await page.goto("/account");
    await update(previousTheme);
    await page.goto(returnTo.href);
  };
}
