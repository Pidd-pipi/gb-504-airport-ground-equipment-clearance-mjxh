import { CommonModule } from '@angular/common';
import { Component, EventEmitter, Input, Output } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';
import { ClearanceDecision } from '../../types';
import { StatusBadgeComponent } from './status-badge.component';
import { EvidenceListComponent } from './evidence-list.component';

@Component({
  selector: 'app-clearance-panel',
  standalone: true,
  imports: [CommonModule, MatIconModule, StatusBadgeComponent, EvidenceListComponent],
  template: `
    <section class="panel" [class.compact]="compact">
      <header><span><mat-icon>verified_user</mat-icon>放行状态</span><app-status-badge [value]="decision?.state || 'pending'"></app-status-badge></header>
      <div *ngIf="decision; else awaiting" class="body">
        <p><strong>决定依据</strong>{{ decision.reason || '等待放行员填写决定依据' }}</p>
        <p *ngIf="decision.restrictions"><strong>运行限制</strong>{{ decision.restrictions }}</p>
        <app-evidence-list [items]="decision.evidence || []"></app-evidence-list>
        <div class="reconsider-trace" *ngIf="decision.reconsidered_from_at">
          <mat-icon>history</mat-icon>
          <span>
            <strong>本次由撤销复议退回</strong>
            原撤销于 {{ decision.reconsidered_from_at | date:'yyyy-MM-dd HH:mm' }}<ng-container *ngIf="decision.reconsidered_from_reason">：{{ decision.reconsidered_from_reason }}</ng-container>
            <small *ngIf="decision.reconsidered_from_request_id">原撤销请求号 {{ decision.reconsidered_from_request_id }}</small>
          </span>
        </div>
        <div class="reconsider-entry" *ngIf="showReconsider && decision.state === 'revoked'">
          <ng-container *ngIf="reconsiderReasons === null"><p class="reconsider-loading">正在核对设备恢复与检查闭环情况…</p></ng-container>
          <ng-container *ngIf="reconsiderReasons && reconsiderReasons.length">
            <p class="reconsider-blocked" *ngFor="let reason of reconsiderReasons"><mat-icon>block</mat-icon>{{ reason }}</p>
          </ng-container>
          <button type="button" class="reconsider-button" [disabled]="!reconsiderReasons || reconsiderReasons.length > 0" (click)="reconsiderRequested.emit()">
            <mat-icon>undo</mat-icon>发起复议，退回待决定
          </button>
        </div>
      </div>
      <ng-template #awaiting><p class="awaiting">检查完成后由安全放行员形成决定。</p></ng-template>
    </section>
  `,
  styles: [`
    .panel { border: 1px solid #d7e1e3; border-left: 3px solid #0e9187; background: #f8fbfb; border-radius: 5px; }
    header { min-height: 48px; padding: 0 14px; display: flex; align-items: center; justify-content: space-between; border-bottom: 1px solid #e0e7e8; }
    header > span { display: flex; align-items: center; gap: 7px; font-weight: 600; color: #273940; }
    header mat-icon { color: #0d8b82; font-size: 19px; width: 19px; height: 19px; }
    .body { padding: 12px 14px; } p { margin: 0 0 10px; color: #576a71; font-size: 12px; line-height: 1.5; }
    p strong { display: block; color: #263a41; margin-bottom: 2px; }
    .awaiting { padding: 13px 14px; margin: 0; }
    .compact header { min-height: 40px; } .compact .body { padding: 9px 12px; }
    .reconsider-trace { display: flex; gap: 8px; margin-top: 4px; padding: 9px 10px; border: 1px dashed #b9cfcd; border-radius: 4px; background: #eef6f4; color: #4c6269; font-size: 11px; line-height: 1.6; }
    .reconsider-trace mat-icon { flex: none; color: #0d8b82; font-size: 17px; width: 17px; height: 17px; margin-top: 1px; }
    .reconsider-trace strong { display: block; color: #2c4a44; }
    .reconsider-trace small { display: block; color: #7d8f94; font-size: 10px; }
    .reconsider-entry { margin-top: 10px; padding-top: 10px; border-top: 1px solid #e0e7e8; }
    .reconsider-loading { margin: 0 0 8px; color: #7d8f94; font-size: 11px; }
    .reconsider-blocked { display: flex; align-items: center; gap: 6px; margin: 0 0 6px; color: #a63b32; font-size: 11px; }
    .reconsider-blocked mat-icon { flex: none; font-size: 15px; width: 15px; height: 15px; }
    .reconsider-button { width: 100%; min-height: 36px; display: inline-flex; align-items: center; justify-content: center; gap: 6px; border: 1px solid #0d8b82; border-radius: 4px; background: #fff; color: #0d8b82; font-size: 12px; font-weight: 600; cursor: pointer; }
    .reconsider-button mat-icon { font-size: 17px; width: 17px; height: 17px; }
    .reconsider-button:disabled { border-color: #c3ced0; color: #9aa8ac; cursor: not-allowed; }
  `],
})
export class ClearancePanelComponent {
  @Input() decision: ClearanceDecision | null = null;
  @Input() compact = false;
  /** Whether the revoked state offers a reconsideration entry (role-gated by the page). */
  @Input() showReconsider = false;
  /** Null while blockers load; empty list means reconsideration is available. */
  @Input() reconsiderReasons: string[] | null = null;
  @Output() reconsiderRequested = new EventEmitter<void>();
}
