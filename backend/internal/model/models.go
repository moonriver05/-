package model

import (
	"time"

	"gorm.io/gorm"
)

type Article struct {
	ID              uint           `json:"id" gorm:"primaryKey"`
	PageID          int64          `json:"pageId" gorm:"uniqueIndex;not null"`
	RevisionID      int64          `json:"revisionId" gorm:"index;not null"`
	Title           string         `json:"title" gorm:"size:255;index;not null"`
	SourceTitle     string         `json:"sourceTitle" gorm:"size:255"`
	Slug            string         `json:"slug" gorm:"size:255;uniqueIndex;not null"`
	Lead            string         `json:"excerpt" gorm:"type:text"`
	PlainText       string         `json:"-" gorm:"type:longtext"`
	RawWikitext     string         `json:"-" gorm:"type:longtext"`
	Category        string         `json:"category" gorm:"size:255;index"`
	Categories      string         `json:"-" gorm:"type:json"`
	Tags            string         `json:"-" gorm:"type:json"`
	Kind            string         `json:"kind" gorm:"size:24;index;default:概念"`
	Difficulty      string         `json:"difficulty" gorm:"size:24;index"`
	ReadMinutes     int            `json:"readMinutes"`
	Language        string         `json:"language" gorm:"size:16;default:zh-Hans;index"`
	SourceURL       string         `json:"sourceUrl" gorm:"size:768"`
	License         string         `json:"license" gorm:"size:64;default:CC BY-SA 4.0"`
	ReviewStatus    string         `json:"reviewStatus" gorm:"size:32;index;default:machine_selected"`
	ContentVersion  int            `json:"contentVersion"`
	ContentWarnings string         `json:"-" gorm:"type:longtext"`
	Published       bool           `json:"published" gorm:"index;default:false"`
	Sections        []Section      `json:"sections,omitempty" gorm:"constraint:OnDelete:CASCADE"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
	DeletedAt       gorm.DeletedAt `json:"-" gorm:"index"`
}

type Section struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	ArticleID   uint      `json:"articleId" gorm:"index;not null"`
	Position    int       `json:"position" gorm:"index"`
	Title       string    `json:"title" gorm:"type:longtext"`
	Content     string    `json:"content" gorm:"type:longtext"`
	ContentHTML string    `json:"contentHtml" gorm:"type:longtext"`
	Level       int       `json:"level"`
	Hidden      bool      `json:"-" gorm:"default:false"`
	Formulae    string    `json:"formulae,omitempty" gorm:"type:json"`
	CreatedAt   time.Time `json:"-"`
	UpdatedAt   time.Time `json:"-"`
}

type User struct {
	Email           *string    `json:"-" gorm:"size:254;uniqueIndex"`
	EmailVerifiedAt *time.Time `json:"-"`
	ID              uint       `json:"id" gorm:"primaryKey"`
	Username        *string    `json:"username,omitempty" gorm:"size:40;uniqueIndex"`
	PasswordHash    string     `json:"-" gorm:"size:255"`
	Name            string     `json:"name" gorm:"size:80;not null"`
	Avatar          string     `json:"avatar" gorm:"size:512"`
	Bio             string     `json:"bio" gorm:"size:1600"`
	Location        string     `json:"location" gorm:"size:240"`
	Website         string     `json:"website" gorm:"size:512"`
	Role            string     `json:"role" gorm:"size:24;default:learner"`
	Banned          bool       `json:"banned" gorm:"not null;default:false"`
	CreatedAt       time.Time  `json:"createdAt"`
}

// Only a hash of the random session token is stored, never the cookie itself.
type Session struct {
	ID        string    `gorm:"primaryKey;size:64"`
	UserID    uint      `gorm:"index;not null"`
	ExpiresAt time.Time `gorm:"index;not null"`
}

// Only COS metadata is stored here; original uploads are never retained.
type UserAvatar struct {
	UserID    uint   `gorm:"primaryKey"`
	Version   string `gorm:"size:32;not null"`
	ObjectKey string `json:"-" gorm:"size:512;not null"`
}

type EmailVerification struct {
	Email       string `gorm:"primaryKey;size:254"`
	CodeHash    string `gorm:"size:255"`
	Ready       bool
	ExpiresAt   time.Time
	SentAt      time.Time
	Attempts    int
	WindowStart time.Time
	SendCount   int
}

type ArticleComment struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	ArticleID uint           `json:"articleId" gorm:"index;not null"`
	UserID    uint           `json:"userId" gorm:"index;not null"`
	Body      string         `json:"body" gorm:"type:text;not null"`
	User      User           `json:"author"`
	CreatedAt time.Time      `json:"createdAt"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

type Annotation struct {
	ID          uint           `json:"id" gorm:"primaryKey"`
	ArticleID   uint           `json:"articleId" gorm:"index;not null"`
	SectionID   *uint          `json:"sectionId" gorm:"index"`
	UserID      uint           `json:"userId" gorm:"index;not null"`
	Quote       string         `json:"quote" gorm:"type:text;not null"`
	Prefix      string         `json:"prefix" gorm:"size:255"`
	Suffix      string         `json:"suffix" gorm:"size:255"`
	StartOffset int            `json:"startOffset"`
	EndOffset   int            `json:"endOffset"`
	Body        string         `json:"body" gorm:"type:text;not null"`
	Color       string         `json:"color" gorm:"size:16;default:#f6c85f"`
	Status      string         `json:"status" gorm:"size:24;index;default:visible"`
	Likes       int            `json:"likes"`
	User        User           `json:"author"`
	Comments    []Comment      `json:"comments,omitempty" gorm:"constraint:OnDelete:CASCADE"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	DeletedAt   gorm.DeletedAt `json:"-" gorm:"index"`
}

type Comment struct {
	ID           uint           `json:"id" gorm:"primaryKey"`
	AnnotationID uint           `json:"annotationId" gorm:"index;not null"`
	UserID       uint           `json:"userId" gorm:"index;not null"`
	ParentID     *uint          `json:"parentId" gorm:"index"`
	Body         string         `json:"body" gorm:"type:text;not null"`
	Status       string         `json:"status" gorm:"size:24;index;default:visible"`
	User         User           `json:"author"`
	CreatedAt    time.Time      `json:"createdAt"`
	DeletedAt    gorm.DeletedAt `json:"-" gorm:"index"`
}

type ExplanationRecord struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	ArticleID  uint      `json:"articleId" gorm:"index;not null"`
	UserID     uint      `json:"userId" gorm:"index"`
	Question   string    `json:"question" gorm:"type:text"`
	Audience   string    `json:"audience" gorm:"size:40"`
	Style      string    `json:"style" gorm:"size:40"`
	AnswerJSON string    `json:"-" gorm:"type:json"`
	Model      string    `json:"model" gorm:"size:80"`
	Confidence float64   `json:"confidence"`
	CreatedAt  time.Time `json:"createdAt"`
}

type Feedback struct {
	Status        string     `json:"status" gorm:"size:24;index;default:pending"`
	Resolution    string     `json:"resolution" gorm:"type:text"`
	ReviewerID    *uint      `json:"reviewerId,omitempty"`
	ReviewedAt    *time.Time `json:"reviewedAt,omitempty"`
	ID            uint       `json:"id" gorm:"primaryKey"`
	UserID        uint       `json:"userId" gorm:"index"`
	ArticleID     *uint      `json:"articleId" gorm:"index"`
	ExplanationID *uint      `json:"explanationId" gorm:"index"`
	Kind          string     `json:"kind" gorm:"size:40;index"`
	Body          string     `json:"body" gorm:"type:text"`
	CreatedAt     time.Time  `json:"createdAt"`
}

type Favorite struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"userId" gorm:"uniqueIndex:idx_user_article;not null"`
	ArticleID uint      `json:"articleId" gorm:"uniqueIndex:idx_user_article;not null"`
	CreatedAt time.Time `json:"createdAt"`
}

type ReadingHistory struct {
	ID        uint `gorm:"primaryKey"`
	UserID    uint `gorm:"uniqueIndex:idx_read_user_article;not null"`
	ArticleID uint `gorm:"uniqueIndex:idx_read_user_article;not null"`
	UpdatedAt time.Time
}

type Category struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"size:80;uniqueIndex;not null"`
	CreatedAt time.Time `json:"createdAt"`
}

func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&Article{}, &Section{}, &User{}, &Session{}, &UserAvatar{}, &EmailVerification{}, &Annotation{}, &Comment{}, &ArticleComment{}, &ExplanationRecord{}, &Feedback{}, &Favorite{}, &ReadingHistory{}, &Category{}, &AnnotationLike{}, &CommunitySubmission{}, &PasswordReset{}, &AdminAudit{})
}

type AnnotationLike struct {
	AnnotationID uint `gorm:"primaryKey"`
	UserID       uint `gorm:"primaryKey"`
}
type CommunitySubmission struct {
	ID          uint      `gorm:"primaryKey"`
	UserID      uint      `gorm:"index:idx_submission_user_time"`
	Fingerprint string    `gorm:"size:64"`
	CreatedAt   time.Time `gorm:"index:idx_submission_user_time"`
}
type PasswordReset struct{ EmailVerification }
type AdminAudit struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	UserID    uint      `json:"userId" gorm:"index"`
	Action    string    `json:"action" gorm:"size:255"`
	Target    string    `json:"target" gorm:"size:40"`
	Reason    string    `json:"reason" gorm:"size:800"`
	Status    int       `json:"status"`
	CreatedAt time.Time `json:"createdAt" gorm:"index"`
}
