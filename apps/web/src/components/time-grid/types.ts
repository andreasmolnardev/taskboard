export interface TimeGridEvent {
  id: string;
  title: string;
  start: Date;
  end: Date;
  allDay?: boolean;
  description?: string;
  color?: string;
}

export interface DateRange {
  start: Date;
  end: Date;
}

export interface EventColumn {
  event: TimeGridEvent;
  column: number;
  columnCount: number;
}

export interface EventSegment {
  event: TimeGridEvent;
  day: Date;
  start: Date;
  end: Date;
  dayIndex: number;
  startsBeforeDay: boolean;
  endsAfterDay: boolean;
}

export interface MinutePosition {
  top: number;
  height: number;
}

export interface CurrentTimePosition {
  dayIndex: number;
  minute: number;
  top: number;
  visible: boolean;
}
