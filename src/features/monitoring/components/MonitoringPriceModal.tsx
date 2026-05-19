import type { TFunction } from 'i18next';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { Modal } from '@/components/ui/Modal';
import { Select, type SelectOption } from '@/components/ui/Select';
import type { ModelPrice } from '@/utils/usage';
import styles from '../MonitoringCenterPage.module.scss';

type PriceDraft = {
  prompt: string;
  completion: string;
  cache: string;
};

type PriceDraftField = keyof PriceDraft;

type MonitoringPriceModalProps = {
  open: boolean;
  priceModel: string;
  priceModelOptions: ReadonlyArray<SelectOption>;
  priceDraft: PriceDraft;
  savedPriceEntries: Array<[string, ModelPrice]>;
  syncingPrices: boolean;
  t: TFunction;
  onClose: () => void;
  onPriceModelChange: (model: string) => void;
  onPriceDraftChange: (field: PriceDraftField, value: string) => void;
  onSyncModelPrices: () => void;
  onResetPriceEditor: () => void;
  onSavePrice: () => void;
  onDeletePrice: (model: string) => void;
};

const formatPriceUnit = (value: number) => `$${value.toFixed(4)}/1M`;

export function MonitoringPriceModal({
  open,
  priceModel,
  priceModelOptions,
  priceDraft,
  savedPriceEntries,
  syncingPrices,
  t,
  onClose,
  onPriceModelChange,
  onPriceDraftChange,
  onSyncModelPrices,
  onResetPriceEditor,
  onSavePrice,
  onDeletePrice,
}: MonitoringPriceModalProps) {
  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('usage_stats.model_price_settings')}
      width={860}
      className={styles.monitorModal}
    >
      <div className={styles.priceEditor}>
        <div className={styles.priceGrid}>
          <div className={`${styles.priceField} ${styles.priceFieldModel}`}>
            <label>{t('usage_stats.model_name')}</label>
            <Select
              value={priceModel}
              options={priceModelOptions}
              onChange={onPriceModelChange}
              ariaLabel={t('usage_stats.model_name')}
            />
          </div>
          <div className={`${styles.priceField} ${styles.priceFieldPrompt}`}>
            <label>{`${t('usage_stats.model_price_prompt')} ($/1M)`}</label>
            <Input
              type="number"
              value={priceDraft.prompt}
              onChange={(event) => onPriceDraftChange('prompt', event.target.value)}
              placeholder="0.0000"
              step="0.0001"
            />
          </div>
          <div className={`${styles.priceField} ${styles.priceFieldCompletion}`}>
            <label>{`${t('usage_stats.model_price_completion')} ($/1M)`}</label>
            <Input
              type="number"
              value={priceDraft.completion}
              onChange={(event) => onPriceDraftChange('completion', event.target.value)}
              placeholder="0.0000"
              step="0.0001"
            />
          </div>
          <div className={`${styles.priceField} ${styles.priceFieldCache}`}>
            <label>{`${t('usage_stats.model_price_cache')} ($/1M)`}</label>
            <Input
              type="number"
              value={priceDraft.cache}
              onChange={(event) => onPriceDraftChange('cache', event.target.value)}
              placeholder="0.0000"
              step="0.0001"
            />
          </div>
        </div>

        <div className={styles.priceActionsBar}>
          <Button
            variant="secondary"
            size="sm"
            onClick={onSyncModelPrices}
            loading={syncingPrices}
          >
            {t('usage_stats.model_price_sync')}
          </Button>
          <Button variant="secondary" size="sm" onClick={onResetPriceEditor}>
            {t('common.cancel')}
          </Button>
          <Button variant="primary" size="sm" onClick={onSavePrice} disabled={!priceModel}>
            {t('common.save')}
          </Button>
        </div>
      </div>

      <div className={styles.savedPricesList}>
        <div className={styles.savedPricesHeader}>{t('usage_stats.saved_prices')}</div>
        {savedPriceEntries.length > 0 ? (
          <div className={styles.savedPricesTableWrap}>
            <table className={styles.savedPricesTable}>
              <thead>
                <tr>
                  <th>{t('usage_stats.model_name')}</th>
                  <th>{t('usage_stats.model_price_prompt')}</th>
                  <th>{t('usage_stats.model_price_completion')}</th>
                  <th>{t('usage_stats.model_price_cache')}</th>
                  <th>{t('common.action')}</th>
                </tr>
              </thead>
              <tbody>
                {savedPriceEntries.map(([model, price]) => (
                  <tr key={model}>
                    <td className={`${styles.monoCell} ${styles.savedPricesModelCell}`}>
                      {model}
                    </td>
                    <td>{formatPriceUnit(price.prompt)}</td>
                    <td>{formatPriceUnit(price.completion)}</td>
                    <td>{formatPriceUnit(price.cache)}</td>
                    <td className={styles.savedPricesActionsCell}>
                      <div className={styles.savedPricesActions}>
                        <button
                          type="button"
                          className={styles.inlineActionButton}
                          onClick={() => onPriceModelChange(model)}
                        >
                          {t('common.edit')}
                        </button>
                        <button
                          type="button"
                          className={styles.inlineActionButton}
                          onClick={() => onDeletePrice(model)}
                        >
                          {t('common.delete')}
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <div className={styles.emptyBlockSmall}>{t('usage_stats.model_price_empty')}</div>
        )}
      </div>
    </Modal>
  );
}
