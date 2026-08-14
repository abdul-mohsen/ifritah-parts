import { expect, test } from '@playwright/test';

test.describe('Story 1 regression safety net', () => {
  test('VIN flow can open a matched vehicle in catalog', async ({ page }) => {
    await page.route('**/api/vin/decode', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          vin: 'KM8J33A46GU123456',
          nhtsaRaw: {
            make: 'HYUNDAI',
            model: 'TUCSON',
            modelYear: '2017',
            fuelType: 'Gasoline',
            engineDisplacementCC: '1999',
          },
          vehicle: {
            linkageTargetId: 10001,
            make: 'HYUNDAI',
            model: 'TUCSON',
            description: 'TUCSON 2.0 MPI (TL) 2015-2018',
            fuelType: 'Petrol',
            capacityCC: 1999,
            horsePower: 155,
          },
          allVariants: [
            {
              linkageTargetId: 10001,
              make: 'HYUNDAI',
              model: 'TUCSON',
              description: 'TUCSON 2.0 MPI (TL) 2015-2018',
              fuelType: 'Petrol',
              capacityCC: 1999,
              horsePower: 155,
            },
            {
              linkageTargetId: 10002,
              make: 'HYUNDAI',
              model: 'TUCSON',
              description: 'TUCSON 1.6 T-GDI (TL) 2015-2018',
              fuelType: 'Petrol',
              capacityCC: 1591,
              horsePower: 177,
            },
          ],
          needsConfirmation: true,
        }),
      });
    });

    await page.goto('/');
    await page.getByLabel('VIN (17 characters)').fill('KM8J33A46GU123456');
    await page.getByRole('button', { name: 'Decode' }).click();

    await expect(page.getByTestId('vin-catalog-matches')).toBeVisible();
    await expect(page.getByTestId('vin-match-card')).toHaveCount(2);

    await page.getByTestId('vin-open-catalog').first().click();

    await expect(page).toHaveURL(/\/catalog\?/);
    await expect(page.getByTestId('catalog-source-banner')).toContainText('KM8J33A46GU123456');
    await expect(page.getByText('TUCSON 2.0 MPI (TL) 2015-2018')).toBeVisible();
  });

  test('confirmed VIN context carries part-name search into the constrained search flow', async ({ page }) => {
    await page.route('**/api/vin/decode', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          vin: 'KM8J33A46GU123456',
          nhtsaRaw: { make: 'HYUNDAI', model: 'TUCSON', modelYear: '2016' },
          vehicle: {
            linkageTargetId: 10001,
            make: 'HYUNDAI',
            model: 'TUCSON',
            description: 'TUCSON 2.0 MPI (TL) 2015-2018',
            fuelType: 'Petrol',
            capacityCC: 1999,
          },
          allVariants: [
            {
              linkageTargetId: 10001,
              make: 'HYUNDAI',
              model: 'TUCSON',
              description: 'TUCSON 2.0 MPI (TL) 2015-2018',
              fuelType: 'Petrol',
              capacityCC: 1999,
            },
            {
              linkageTargetId: 10002,
              make: 'HYUNDAI',
              model: 'TUCSON',
              description: 'TUCSON 1.6 T-GDI (TL) 2015-2018',
              fuelType: 'Petrol',
              capacityCC: 1591,
            },
          ],
          needsConfirmation: true,
        }),
      });
    });

    await page.goto('/');
    await page.getByLabel('VIN (17 characters)').fill('KM8J33A46GU123456');
    await page.getByRole('button', { name: 'Decode' }).click();
    await page.getByRole('button', { name: 'Use here' }).first().click();

    await expect(page.getByTestId('vin-part-search')).toBeVisible();
    await page.getByLabel('Part name for confirmed vehicle').fill('cabin air filter');
    await page.getByRole('button', { name: 'Search this vehicle' }).click();

    await expect(page).toHaveURL(/\/oem\?.*vehicleId=10001/);
    await expect(page.getByTestId('search-vehicle-context')).toContainText('HYUNDAI TUCSON');
    await expect(page.getByLabel('OEM / Part Number / Description')).toHaveValue('cabin air filter');
  });

  test('VIN recall notice shows its NHTSA source and non-VIN-specific scope warning', async ({ page }) => {
    await page.route('**/api/vin/decode', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          vin: 'KNDPMCAC4J7412345',
          nhtsaRaw: { make: 'KIA', model: 'Sportage', modelYear: '2018' },
          recalls: [{
            nhtsaCampaignNumber: '21V137000',
            component: 'SERVICE BRAKES',
            summary: 'The Hydraulic Electronic Control Unit may short-circuit.',
            sourceLabel: 'NHTSA vehicle recall API',
            sourceUrl: 'https://api.nhtsa.gov/recalls/recallsByVehicle?make=KIA&model=SPORTAGE&modelYear=2018',
            warning: 'NHTSA recall results are matched by make, model, and model year. They do not confirm that this exact VIN is affected or that a remedy remains open.',
          }],
        }),
      });
    });

    await page.goto('/');
    await page.getByLabel('VIN (17 characters)').fill('KNDPMCAC4J7412345');
    await page.getByRole('button', { name: 'Decode' }).click();

    const recallNotice = page.getByTestId('recall-banner');
    await expect(recallNotice).toContainText('NHTSA Safety Recall Notice');
    await expect(recallNotice).toContainText('21V137000');
    await expect(recallNotice).toContainText('do not confirm that this exact VIN is affected');
    await expect(recallNotice.getByRole('link', { name: 'NHTSA vehicle recall API' })).toHaveAttribute('href', /api\.nhtsa\.gov\/recalls/);
  });

  test('search UI deduplicates repeated results for 97133', async ({ page }) => {
    await page.goto('/oem');
    await page.getByLabel('OEM / Part Number / Description').fill('97133');
    await page.getByRole('button', { name: 'Search' }).click();

    await expect(page.getByTestId('search-result-card').first()).toBeVisible();
    const articleNumbers = await page.getByTestId('search-result-article').allInnerTexts();
    expect(articleNumbers.length).toBeGreaterThan(0);
    expect(new Set(articleNumbers).size).toBe(articleNumbers.length);
  });

  test('part-name search returns cabin filters without category-only false positives', async ({ page }) => {
    await page.goto('/oem');
    await page.getByLabel('OEM / Part Number / Description').fill('cabin air filter');
    await page.getByRole('button', { name: 'Search' }).click();

    await expect(page.getByTestId('search-result-card').first()).toBeVisible();
    const articleNumbers = await page.getByTestId('search-result-article').allInnerTexts();
    expect(articleNumbers).toContain('97133-D3000');
    expect(articleNumbers).toContain('CUK 26 013');
    expect(articleNumbers).not.toContain('97113-D3000');
    expect(articleNumbers).not.toContain('97115-D3000');
    await expect(page.getByText('Heater Core', { exact: true })).toHaveCount(0);
    await expect(page.getByText('Blower Motor', { exact: true })).toHaveCount(0);
  });

  test('OEM search can hand off into catalog with vehicle context', async ({ page }) => {
    await page.goto('/oem');
    await page.getByLabel('OEM / Part Number / Description').fill('97133-D3000');
    await page.getByRole('button', { name: 'Search' }).click();

    const tucsonVehicle = page.getByTestId('oem-fit-vehicle').filter({ hasText: 'TUCSON' }).first();
    await expect(tucsonVehicle).toBeVisible();
    await tucsonVehicle.click();

    await expect(page).toHaveURL(/\/catalog\?/);
    await expect(page.getByTestId('catalog-source-banner')).toContainText('97133-D3000');
    await expect(page).toHaveURL(/model=TUCSON/);
  });

  test('catalog can still open part detail modal', async ({ page }) => {
    await page.goto('/catalog?make=HYUNDAI&model=TUCSON&vehicleId=10001&sourceType=e2e&sourceQuery=story1');

    await page.getByTestId('catalog-all-parts').click();
    await expect(page.getByTestId('catalog-part-article').first()).toBeVisible();
    await page.getByTestId('catalog-part-article').first().click();

    await expect(page.getByText('Evidence-first part detail with provenance, fitment context, replacement guidance, and only real media when available.')).toBeVisible();
    await expect(page.getByText(/^Source:/)).toBeVisible();
    await expect(page.getByText(/^Confidence:/)).toBeVisible();
    await expect(page.getByTestId('missing-specification-guidance')).toContainText('A category or description match is not evidence that those details match.');
  });
});
