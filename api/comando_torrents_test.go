package handler

import (
	"reflect"
	"testing"
	"time"
)

func Test_processTitle(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{
			name:  "should remove the download suffix",
			title: "Moana – Download",
			want:  "Moana",
		},
		{
			name:  "should remove the website name",
			title: "Moana comando.la",
			want:  "Moana",
		},
		{
			name:  "should keep the release title as is",
			title: "Moana.2026.WEB-DL.1080p.DUAL.5.1",
			want:  "Moana.2026.WEB-DL.1080p.DUAL.5.1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := processTitle(tt.title); got != tt.want {
				t.Errorf("processTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func Test_parseLocalizedDate(t *testing.T) {
	type args struct {
		datePublished string
	}
	tests := []struct {
		name    string
		args    args
		want    time.Time
		wantErr bool
	}{
		{
			name: "should return date",
			args: args{
				datePublished: "12 de outubro de 2022",
			},
			want:    time.Date(2022, 10, 12, 0, 0, 0, 0, time.UTC),
			wantErr: false,
		},
		{
			name: "should return date single digit",
			args: args{
				datePublished: "1 de outubro de 2022",
			},
			want:    time.Date(2022, 10, 1, 0, 0, 0, 0, time.UTC),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLocalizedDate(tt.args.datePublished)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseDate() = %v, want %v", got, tt.want)
			}
		})
	}
}
